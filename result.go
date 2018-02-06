package pgwire

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/mcuadros/pgwire/basesql"
	"github.com/mcuadros/pgwire/datum"
	"github.com/mcuadros/pgwire/pgerror"
	"github.com/mcuadros/pgwire/pgwirebase"
)

// StatementResult is used to produce results for a single query (see
// ResultsWriter).
type StatementResult interface {
	// BeginResult should be called prior to any of the other methods.
	// TODO(andrei): remove BeginResult and SetColumns, and have
	// NewStatementResult() take in a tree.Statement
	BeginResult(typ basesql.StatementType, tag basesql.StatementTag)
	// GetPGTag returns the PGTag of the statement passed into BeginResult.
	StatementTag() basesql.StatementTag
	// GetStatementType returns the StatementType that corresponds to the type of
	// results that should be sent to this interface.
	StatementType() basesql.StatementType
	// SetColumns should be called after BeginResult and before AddRow if the
	// StatementType is tree.Rows.
	SetColumns(columns ResultColumns)
	// AddRow takes the passed in row and adds it to the current result.
	AddRow(ctx context.Context, row datum.Datums) error
	// IncrementRowsAffected increments a counter by n. This is used for all
	// result types other than tree.Rows.
	IncrementRowsAffected(n int)
	// RowsAffected returns either the number of times AddRow was called, or the
	// sum of all n passed into IncrementRowsAffected.
	RowsAffected() int
	// CloseResult ends the current result. The v3Conn will send control codes to
	// the client informing it that the result for a statement is now complete.
	//
	// CloseResult cannot be called unless there's a corresponding BeginResult
	// prior.
	CloseResult() error
	// SetError allows an error to be stored on the StatementResult.
	SetError(err error)
	// Err returns the error previously set with SetError(), if any.
	Err() error
}

type statementResult struct {
	state *streamingState
	conn  *v3Conn

	tagBuf     [64]byte
	curStmtErr error
}

func NewStatementResult(s *streamingState, conn *v3Conn) StatementResult {
	return &statementResult{state: s, conn: conn}
}

// BeginResult implements the StatementResult interface.
func (r *statementResult) BeginResult(typ basesql.StatementType, tag basesql.StatementTag) {
	r.state.pgTag = tag
	r.state.statementType = typ
	r.state.rowsAffected = 0
	r.state.firstRow = true
}

func (r *statementResult) StatementTag() basesql.StatementTag {
	return r.state.pgTag
}

func (r *statementResult) StatementType() basesql.StatementType {
	return r.state.statementType
}

func (r *statementResult) SetColumns(columns ResultColumns) {
	r.state.columns = columns
}

func (r *statementResult) AddRow(ctx context.Context, row datum.Datums) error {
	if r.state.err != nil {
		return r.state.err
	}

	if r.state.statementType != basesql.Rows {
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

	r.conn.writeBuf.initMsg(pgwirebase.ServerMsgDataRow)
	r.conn.writeBuf.putInt16(int16(len(row)))
	for i, col := range row {
		fmtCode := pgwirebase.FormatText
		if formatCodes != nil {
			fmtCode = formatCodes[i]
		}
		switch fmtCode {
		case pgwirebase.FormatText:
			r.conn.writeBuf.writeTextDatum(ctx, col, r.conn.session.Location())
		case pgwirebase.FormatBinary:
			r.conn.writeBuf.writeBinaryDatum(ctx, col, r.conn.session.Location())
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
	case basesql.RowsAffected:
		tag = append(tag, ' ')
		tag = strconv.AppendInt(tag, int64(r.state.rowsAffected), 10)
		return r.sendCommandComplete(tag, &r.state.buf)

	case basesql.Rows:
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

	case basesql.Ack, basesql.DDL:
		if r.state.pgTag == "SELECT" {
			tag = append(tag, ' ')
			tag = strconv.AppendInt(tag, int64(r.state.rowsAffected), 10)
		}
		return r.sendCommandComplete(tag, &r.state.buf)

	case basesql.CopyIn:
		// Nothing to do. The CommandComplete message has been sent elsewhere.
		panic(fmt.Sprintf("CopyIn statements should have been handled elsewhere " +
			"and not produce results"))
	default:
		panic(fmt.Sprintf("unexpected result type %v", r.state.statementType))
	}
}

func (r *statementResult) sendCommandComplete(tag []byte, w io.Writer) error {
	r.conn.writeBuf.initMsg(pgwirebase.ServerMsgCommandComplete)
	r.conn.writeBuf.write(tag)
	r.conn.writeBuf.nullTerminate()
	return r.conn.writeBuf.finishMsg(w)
}

// sendRowDescription sends a row description over the wire for the given
// slice of columns.
func (r *statementResult) sendRowDescription(
	ctx context.Context,
	columns []ResultColumn,
	formatCodes []pgwirebase.FormatCode,
	w io.Writer,
) error {
	r.conn.writeBuf.initMsg(pgwirebase.ServerMsgRowDescription)
	r.conn.writeBuf.putInt16(int16(len(columns)))
	for i, column := range columns {
		log.Debugf("pgwire: writing column %s of type: %T", column.Name, column.Typ)
		r.conn.writeBuf.writeTerminatedString(column.Name)

		typ := pgTypeForParserType(column.Typ)
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
			r.conn.writeBuf.putInt16(int16(pgwirebase.FormatText))
		} else {
			r.conn.writeBuf.putInt16(int16(formatCodes[i]))
		}
	}
	return r.conn.writeBuf.finishMsg(w)
}
