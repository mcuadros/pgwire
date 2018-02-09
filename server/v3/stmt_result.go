package v3

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gopkg.in/sqle/sqle.v0/sql"

	"github.com/mcuadros/pgwire"
	"github.com/mcuadros/pgwire/pgerror"
)

type statementResult struct {
	state *streamingState
	conn  *v3Conn

	tagBuf     [64]byte
	curStmtErr error
}

func NewStatementResult(s *streamingState, conn *v3Conn) pgwire.StatementResult {
	return &statementResult{state: s, conn: conn}
}

// BeginResult implements the StatementResult interface.
func (r *statementResult) BeginResult(typ pgwire.StatementType, tag pgwire.StatementTag) {
	r.state.pgTag = tag
	r.state.statementType = typ
	r.state.rowsAffected = 0
	r.state.firstRow = true
}

func (r *statementResult) StatementTag() pgwire.StatementTag {
	return r.state.pgTag
}

func (r *statementResult) StatementType() pgwire.StatementType {
	return r.state.statementType
}

func (r *statementResult) SetColumns(columns sql.Schema) {
	r.state.columns = columns
}

func (r *statementResult) AddRow(ctx context.Context, row pgwire.Datums) error {
	if r.state.err != nil {
		return r.state.err
	}

	if r.state.statementType != pgwire.Rows {
		// it was v3.setError
		return r.conn.setError(pgerror.NewError(
			pgerror.CodeInternalError, "cannot use AddRow() with statements that don't return rows"))
	}

	// The final tag will need to know the total row count.
	r.state.rowsAffected++

	formatCodes := r.state.formatCodes

	// First row and description needed: do it.
	if r.state.firstRow && r.state.sendDescription {
		if err := r.sendRowDescription(ctx, r.state.columns, formatCodes, &r.state.buf); err != nil {
			return err
		}
	}
	r.state.firstRow = false

	r.conn.writeBuf.initMsg(pgwire.ServerMsgDataRow)
	r.conn.writeBuf.putInt16(int16(len(row)))
	for i, col := range row {
		fmtCode := pgwire.FormatText
		if formatCodes != nil {
			fmtCode = formatCodes[i]
		}
		switch fmtCode {
		case pgwire.FormatText:
			r.conn.writeBuf.writeTextpgwire(ctx, col, r.conn.session.Location())
		case pgwire.FormatBinary:
			r.conn.writeBuf.writeBinarypgwire(ctx, col, r.conn.session.Location())
		default:
			r.conn.writeBuf.setError(errors.Errorf("unsupported format code %s", fmtCode))
		}
	}

	if err := r.conn.writeBuf.finishMsg(&r.state.buf); err != nil {
		return err
	}

	return r.conn.flush(false /* forceSend */)
}

func (r *statementResult) IncrementRowsAffected(n int) {
	r.state.rowsAffected += n
}

func (r *statementResult) RowsAffected() int {
	return r.state.rowsAffected
}

func (r *statementResult) SetError(err error) {
	r.curStmtErr = err
}

func (r *statementResult) Err() error {
	return r.curStmtErr
}

func (r *statementResult) CloseResult() error {
	r.curStmtErr = nil

	if r.state.err != nil {
		return r.state.err
	}

	ctx := r.conn.session.Ctx()
	if err := r.conn.flush(false /* forceSend */); err != nil {
		return err
	}

	if r.state.pgTag == "INSERT" {
		// From the postgres docs (49.5. Message Formats):
		// `INSERT oid rows`... oid is the object ID of the inserted row if
		//	rows is 1 and the target table has OIDs; otherwise oid is 0.
		r.state.pgTag = "INSERT 0"
	}

	tag := append(r.tagBuf[:0], r.state.pgTag...)

	switch r.state.statementType {
	case pgwire.RowsAffected:
		tag = append(tag, ' ')
		tag = strconv.AppendInt(tag, int64(r.state.rowsAffected), 10)
		return r.sendCommandComplete(tag, &r.state.buf)

	case pgwire.Rows:
		if r.state.firstRow && r.state.sendDescription {
			if err := r.sendRowDescription(ctx,
				r.state.columns, r.state.formatCodes, &r.state.buf,
			); err != nil {
				return err
			}
		}

		tag = append(tag, ' ')
		tag = strconv.AppendUint(tag, uint64(r.state.rowsAffected), 10)
		return r.sendCommandComplete(tag, &r.state.buf)

	case pgwire.Ack, pgwire.DDL:
		if r.state.pgTag == "SELECT" {
			tag = append(tag, ' ')
			tag = strconv.AppendInt(tag, int64(r.state.rowsAffected), 10)
		}
		return r.sendCommandComplete(tag, &r.state.buf)

	case pgwire.CopyIn:
		// Nothing to do. The CommandComplete message has been sent elsewhere.
		panic(fmt.Sprintf("CopyIn statements should have been handled elsewhere " +
			"and not produce results"))
	default:
		panic(fmt.Sprintf("unexpected result type %v", r.state.statementType))
	}
}

func (r *statementResult) sendCommandComplete(tag []byte, w io.Writer) error {
	r.conn.writeBuf.initMsg(pgwire.ServerMsgCommandComplete)
	r.conn.writeBuf.write(tag)
	r.conn.writeBuf.nullTerminate()
	return r.conn.writeBuf.finishMsg(w)
}

// sendRowDescription sends a row description over the wire for the given
// slice of columns.
func (r *statementResult) sendRowDescription(
	ctx context.Context,
	columns []*sql.Column,
	formatCodes []pgwire.FormatCode,
	w io.Writer,
) error {
	r.conn.writeBuf.initMsg(pgwire.ServerMsgRowDescription)
	r.conn.writeBuf.putInt16(int16(len(columns)))
	for i, column := range columns {
		log.Debugf("pgwire: writing column %s of type: %T", column.Name, column.Type)
		r.conn.writeBuf.writeTerminatedString(column.Name)

		typ := pgTypeForParserType(column.Type)
		r.conn.writeBuf.putInt32(0) // Table OID (optional).
		r.conn.writeBuf.putInt16(0) // Column attribute ID (optional).
		r.conn.writeBuf.putInt32(int32(typ.oid))
		r.conn.writeBuf.putInt16(int16(typ.size))
		// The type modifier (atttypmod) is used to include various extra information
		// about the type being sent. -1 is used for values which don't make use of
		// atttypmod and is generally an acceptable catch-all for those that do.
		// See https://www.postgresql.org/docs/9.6/static/catalog-pg-attribute.html
		// for information on atttypmod. In theory we differ from Postgres by never
		// giving the scale/precision, and by not including the length of a VARCHAR,
		// but it's not clear if any drivers/ORMs depend on this.
		//
		// TODO(justin): It would be good to include this information when possible.
		r.conn.writeBuf.putInt32(-1)
		if formatCodes == nil {
			r.conn.writeBuf.putInt16(int16(pgwire.FormatText))
		} else {
			r.conn.writeBuf.putInt16(int16(formatCodes[i]))
		}
	}
	return r.conn.writeBuf.finishMsg(w)
}
