// Copyright 2015 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package v3

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"bytes"
	"io"

	"github.com/mcuadros/pgwire"
	"github.com/mcuadros/pgwire/pgerror"
	"github.com/mcuadros/pgwire/pgwirebase"

	"github.com/cockroachdb/cockroach/pkg/sql"
	// To move
	"github.com/cockroachdb/cockroach/pkg/security"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/tree"
)

const (
	authOK                int32 = 0
	authCleartextPassword int32 = 3
)

const ErrDraining = "server is not accepting clients"

// connResultsBufferSizeBytes refers to the size of the result set which we
// buffer into memory prior to flushing to the client.
const connResultsBufferSizeBytes = 16 << 10

// readTimeoutConn overloads net.Conn.Read by periodically calling
// checkExitConds() and aborting the read if an error is returned.
type readTimeoutConn struct {
	net.Conn
	checkExitConds func() error
}

func newReadTimeoutConn(c net.Conn, checkExitConds func() error) net.Conn {
	// net.Pipe does not support setting deadlines. See
	// https://github.com/golang/go/blob/go1.7.4/src/net/pipe.go#L57-L67
	//
	// TODO(andrei): starting with Go 1.10, pipes are supposed to support
	// timeouts, so this should go away when we upgrade the compiler.
	if c.LocalAddr().Network() == "pipe" {
		return c
	}
	return &readTimeoutConn{
		Conn:           c,
		checkExitConds: checkExitConds,
	}
}

func (c *readTimeoutConn) Read(b []byte) (int, error) {
	// readTimeout is the amount of time ReadTimeoutConn should wait on a
	// read before checking for exit conditions. The tradeoff is between the
	// time it takes to react to session context cancellation and the overhead
	// of waking up and checking for exit conditions.
	const readTimeout = 150 * time.Millisecond

	// Remove the read deadline when returning from this function to avoid
	// unexpected behavior.
	defer func() { _ = c.SetReadDeadline(time.Time{}) }()
	for {
		if err := c.checkExitConds(); err != nil {
			return 0, err
		}
		if err := c.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
			return 0, err
		}
		n, err := c.Conn.Read(b)
		// Continue if the error is due to timing out.
		if err, ok := err.(net.Error); ok && err.Timeout() {
			continue
		}
		return n, err
	}
}

type v3Conn struct {
	conn     net.Conn
	rd       *bufio.Reader
	wr       *bufio.Writer
	executor pgwire.Executor
	readBuf  pgwirebase.ReadBuffer
	writeBuf *writeBuffer
	session  pgwire.Session

	// The logic governing these guys is hairy, and is not sufficiently
	// specified in documentation. Consult the sources before you modify:
	// https://github.com/postgres/postgres/blob/master/src/backend/tcop/postgres.c
	doingExtendedQueryMessage, ignoreTillSync bool

	// The above comment also holds for this boolean, which can be set to cause
	// the backend to *not* send another ready for query message. This behavior
	// is required when the backend is supposed to drop messages, such as when
	// it gets extra data after an error happened during a COPY operation.
	doNotSendReadyForQuery bool

	streamingState streamingState

	// curStmtErr is the error encountered during the execution of the current SQL
	// statement.
	curStmtErr error
}

type streamingState struct {

	/* Current batch state */

	// formatCodes is an array of which indicates whether each column of a row
	// should be sent as binary or text format. If it is nil then we send as text.
	formatCodes     []pgwirebase.FormatCode
	sendDescription bool
	// limit is a feature of pgwire that we don't really support. We accept it and
	// don't complain as long as the statement produces fewer results than this.
	limit      int
	emptyQuery bool
	err        error

	// hasSentResults is set if any results have been sent on the client
	// connection since the last time Close() or Flush() were called. This is used
	// to back the ResultGroup.ResultsSentToClient() interface.
	hasSentResults bool

	// TODO(tso): this can theoretically be combined with v3conn.writeBuf.
	// Currently we write to write to the v3conn.writeBuf, then we take those
	// bytes and write them to buf. We do this since we need to length prefix
	// each message and this is icky to figure out ahead of time.
	buf bytes.Buffer

	// txnStartIdx is the start of the current transaction in the buf. We keep
	// track of this so that we can reset the current transaction if we retry.
	txnStartIdx int

	/* Current statement state */

	columns       pgwire.ResultColumns
	pgTag         pgwire.StatementTag
	statementType pgwire.StatementType
	rowsAffected  int
	// firstRow is true when we haven't sent a row back in a result of type
	// tree.Rows. We only want to send the description once per result.
	firstRow bool
}

func (s *streamingState) reset(
	formatCodes []pgwirebase.FormatCode, sendDescription bool, limit int,
) {
	s.formatCodes = formatCodes
	s.sendDescription = sendDescription
	s.limit = limit
	s.emptyQuery = false
	s.hasSentResults = false
	s.txnStartIdx = 0
	s.err = nil
	s.buf.Reset()
}

func NewConn(conn net.Conn, executor pgwire.Executor) v3Conn {
	return v3Conn{
		conn:     conn,
		rd:       bufio.NewReader(conn),
		wr:       bufio.NewWriter(conn),
		writeBuf: newWriteBuffer(),
		executor: executor,
	}
}

func (c *v3Conn) Finish(ctx context.Context) {
	// This is better than always flushing on error.
	if err := c.wr.Flush(); err != nil {
		log.Error(ctx, err)
	}
	_ = c.conn.Close()
}

// statusReportParams is a static mapping from run-time parameters to their respective
// hard-coded values, each of which is to be returned as part of the status report
// during connection initialization.
var statusReportParams = map[string]string{
	"client_encoding": "UTF8",
	"DateStyle":       "ISO",
	// All datetime binary formats expect 64-bit integer microsecond values.
	// This param needs to be provided to clients or some may provide 64-bit
	// floating-point microsecond values instead, which was a legacy datetime
	// binary format.
	"integer_datetimes": "on",
	// The latest version of the docs that was consulted during the development
	// of this package. We specify this version to avoid having to support old
	// code paths which various client tools fall back to if they can't
	// determine that the server is new enough.
	"server_version": sql.PgServerVersion,
	// The current CockroachDB version string.
	"crdb_version": "42",
	// If this parameter is not present, some drivers (including Python's psycopg2)
	// will add redundant backslash escapes for compatibility with non-standard
	// backslash handling in older versions of postgres.
	"standard_conforming_strings": "on",
}

// handleAuthentication should discuss with the client to arrange
// authentication and update c.SessionArgs with the authenticated user's
// name, if different from the one given initially. Note: at this
// point the sql.Session does not exist yet! If need exists to access the
// database to look up authentication data, use the internal executor.
func (c *v3Conn) HandleAuthentication(ctx context.Context, insecure bool) error {
	// Check that the requested user exists and retrieve the hashed
	// password in case password authentication is needed.
	var err error
	exists, hashedPassword, err := true, []byte{}, nil
	//TODO: exists, hashedPassword, err := sql.GetUserHashedPassword(
	//	ctx, c.executor, c.metrics.internalMemMetrics, c.SessionArgs.User,
	//)

	if err != nil {
		return c.SendError(err)
	}
	if !exists {
		return c.SendError(errors.Errorf("user %s does not exist", c.session.Args().User))
	}

	if tlsConn, ok := c.conn.(*tls.Conn); ok {
		var authenticationHook security.UserAuthHook

		tlsState := tlsConn.ConnectionState()
		// If no certificates are provided, default to password
		// authentication.
		if len(tlsState.PeerCertificates) == 0 {
			password, err := c.sendAuthPasswordRequest()
			if err != nil {
				return c.SendError(err)
			}
			authenticationHook = security.UserAuthPasswordHook(
				insecure, password, hashedPassword,
			)
		} else {
			// Normalize the username contained in the certificate.
			tlsState.PeerCertificates[0].Subject.CommonName = tree.Name(
				tlsState.PeerCertificates[0].Subject.CommonName,
			).Normalize()
			var err error
			authenticationHook, err = security.UserAuthCertHook(insecure, &tlsState)
			if err != nil {
				return c.SendError(err)
			}
		}

		if err := authenticationHook(c.session.Args().User, true /* public */); err != nil {
			return c.SendError(err)
		}
	}

	c.writeBuf.initMsg(pgwirebase.ServerMsgAuth)
	c.writeBuf.putInt32(authOK)
	return c.writeBuf.finishMsg(c.wr)
}

func (c *v3Conn) setupSession(s pgwire.Session) error {
	c.session = s
	return nil
}

func (c *v3Conn) closeSession(ctx context.Context) {
	c.session.Finish()
	c.session = nil
}

func (c *v3Conn) Serve(ctx context.Context, s pgwire.Session, draining func() bool) error {
	if err := c.setupSession(s); err != nil {
		return err
	}

	for key, value := range statusReportParams {
		c.writeBuf.initMsg(pgwirebase.ServerMsgParameterStatus)
		c.writeBuf.writeTerminatedString(key)
		c.writeBuf.writeTerminatedString(value)
		if err := c.writeBuf.finishMsg(c.wr); err != nil {
			return err
		}
	}
	if err := c.wr.Flush(); err != nil {
		return err
	}

	//	ctx = log.WithLogTagStr(ctx, "user", c.SessionArgs.User)

	// Now that a Session has been set up, further operations done on behalf of
	// this session use Session.Ctx() (which may diverge from this method's ctx).

	defer func() {
		if r := recover(); r != nil {
			// If we're panicking, use an emergency session shutdown so that
			// the monitors don't shout that they are unhappy.
			//TODO c.session.EmergencyClose()
			panic(r)
		}
		c.closeSession(ctx)
	}()

	// Once a session has been set up, the underlying net.Conn is switched to
	// a conn that exits if the session's context is canceled or if the server
	// is draining and the session does not have an ongoing transaction.
	c.conn = newReadTimeoutConn(c.conn, func() error {
		if err := func() error {
			if draining() {
				return errors.New(ErrDraining)
			}
			return c.session.Ctx().Err()
		}(); err != nil {
			return newAdminShutdownErr(err)
		}
		return nil
	})
	c.rd = bufio.NewReader(c.conn)

	for {
		if !c.doingExtendedQueryMessage && !c.doNotSendReadyForQuery {
			c.writeBuf.initMsg(pgwirebase.ServerMsgReady)
			var txnStatus byte
			switch sql.NoTxn { //TODO c.session.TxnState.State() {
			case sql.Aborted, sql.RestartWait:
				// We send status "InFailedTransaction" also for state RestartWait
				// because GO's lib/pq freaks out if we invent a new status.
				txnStatus = 'E'
			case sql.Open, sql.AutoRetry:
				txnStatus = 'T'
			case sql.NoTxn:
				// We're not in a txn (i.e. the last txn was committed).
				txnStatus = 'I'
			case sql.CommitWait:
				// We need to lie to pgwire and claim that we're still
				// in a txn. Otherwise drivers freak out.
				// This state is not part of the Postgres protocol.
				txnStatus = 'T'
			}

			log.Debugf("pgwire: %s: %q", pgwirebase.ServerMsgReady, txnStatus)

			c.writeBuf.writeByte(txnStatus)
			if err := c.writeBuf.finishMsg(c.wr); err != nil {
				return err
			}
			// We only flush on every message if not doing an extended query.
			// If we are, wait for an explicit Flush message. See:
			// http://www.postgresql.org/docs/current/static/protocol-flow.html#PROTOCOL-FLOW-EXT-QUERY.
			if err := c.wr.Flush(); err != nil {
				return err
			}
		}
		c.doNotSendReadyForQuery = false
		typ, _, err := c.readBuf.ReadTypedMsg(c.rd)
		if err != nil {
			return err
		}
		// When an error occurs handling an extended query message, we have to ignore
		// any messages until we get a sync.
		if c.ignoreTillSync && typ != pgwirebase.ClientMsgSync {
			log.Debugf("pgwire: ignoring %s till sync", typ)
			continue
		}
		log.Infof("pgwire: processing %s", typ)
		switch typ {
		case pgwirebase.ClientMsgSync:
			c.doingExtendedQueryMessage = false
			c.ignoreTillSync = false

		case pgwirebase.ClientMsgSimpleQuery:
			c.doingExtendedQueryMessage = false
			err = c.handleSimpleQuery(&c.readBuf)

		case pgwirebase.ClientMsgTerminate:
			return nil

		case pgwirebase.ClientMsgParse:
			c.doingExtendedQueryMessage = true
			err = c.handleParse(&c.readBuf)

		case pgwirebase.ClientMsgDescribe:
			c.doingExtendedQueryMessage = true
			err = c.handleDescribe(c.session.Ctx(), &c.readBuf)

		case pgwirebase.ClientMsgClose:
			c.doingExtendedQueryMessage = true
			err = c.handleClose(c.session.Ctx(), &c.readBuf)

		case pgwirebase.ClientMsgBind:
			c.doingExtendedQueryMessage = true
			err = c.handleBind(c.session.Ctx(), &c.readBuf)

		case pgwirebase.ClientMsgExecute:
			c.doingExtendedQueryMessage = true
			err = c.handleExecute(&c.readBuf)

		case pgwirebase.ClientMsgFlush:
			c.doingExtendedQueryMessage = true
			err = c.wr.Flush()

		case pgwirebase.ClientMsgCopyData, pgwirebase.ClientMsgCopyDone, pgwirebase.ClientMsgCopyFail:
			// We don't want to send a ready for query message here - we're supposed
			// to ignore these messages, per the protocol spec. This state will
			// happen when an error occurs on the server-side during a copy
			// operation: the server will send an error and a ready message back to
			// the client, and must then ignore further copy messages. See
			// https://github.com/postgres/postgres/blob/6e1dd2773eb60a6ab87b27b8d9391b756e904ac3/src/backend/tcop/postgres.c#L4295
			c.doNotSendReadyForQuery = true

		default:
			return c.SendError(pgwirebase.NewUnrecognizedMsgTypeErr(typ))
		}
		if err != nil {
			return err
		}
	}
}

// sendAuthPasswordRequest requests a cleartext password from the client and
// returns it.
func (c *v3Conn) sendAuthPasswordRequest() (string, error) {
	c.writeBuf.initMsg(pgwirebase.ServerMsgAuth)
	c.writeBuf.putInt32(authCleartextPassword)
	if err := c.writeBuf.finishMsg(c.wr); err != nil {
		return "", err
	}
	if err := c.wr.Flush(); err != nil {
		return "", err
	}

	typ, _, err := c.readBuf.ReadTypedMsg(c.rd)
	if err != nil {
		return "", err
	}

	if typ != pgwirebase.ClientMsgPassword {
		return "", errors.Errorf("invalid response to authentication request: %s", typ)
	}

	return c.readBuf.GetString()
}

func (c *v3Conn) handleSimpleQuery(buf *pgwirebase.ReadBuffer) error {
	defer c.session.FinishPlan()
	query, err := buf.GetString()
	if err != nil {
		return err
	}

	c.streamingState.reset(
		nil /* formatCodes */, true /* sendDescription */, 0, /* limit */
	)

	if err := c.executor.ExecuteStatements(c.session, c, query); err != nil {
		if err := c.setError(err); err != nil {
			return err
		}
	}
	return c.done()
}

func (c *v3Conn) handleParse(buf *pgwirebase.ReadBuffer) error {
	return c.SendError(fmt.Errorf("not implemented"))
}

func (c *v3Conn) handleDescribe(ctx context.Context, buf *pgwirebase.ReadBuffer) error {
	return c.SendError(fmt.Errorf("not implemented"))
}

func (c *v3Conn) handleClose(ctx context.Context, buf *pgwirebase.ReadBuffer) error {
	return c.SendError(fmt.Errorf("not implemented"))
}

func (c *v3Conn) handleBind(ctx context.Context, buf *pgwirebase.ReadBuffer) error {
	return c.SendError(fmt.Errorf("not implemented"))
}

func (c *v3Conn) handleExecute(buf *pgwirebase.ReadBuffer) error {
	return c.SendError(fmt.Errorf("not implemented"))
}

func (c *v3Conn) SendError(err error) error {
	c.executor.RecordError(err)
	if c.doingExtendedQueryMessage {
		c.ignoreTillSync = true
	}

	c.writeBuf.initMsg(pgwirebase.ServerMsgErrorResponse)

	c.writeBuf.putErrFieldMsg(pgwirebase.ServerErrFieldSeverity)
	c.writeBuf.writeTerminatedString("ERROR")

	pgErr, ok := pgerror.GetPGCause(err)
	var code string
	if ok {
		code = pgErr.Code
	} else {
		code = pgerror.CodeInternalError
	}

	c.writeBuf.putErrFieldMsg(pgwirebase.ServerErrFieldSQLState)
	c.writeBuf.writeTerminatedString(code)

	if ok && pgErr.Detail != "" {
		c.writeBuf.putErrFieldMsg(pgwirebase.ServerErrFileldDetail)
		c.writeBuf.writeTerminatedString(pgErr.Detail)
	}

	if ok && pgErr.Hint != "" {
		c.writeBuf.putErrFieldMsg(pgwirebase.ServerErrFileldHint)
		c.writeBuf.writeTerminatedString(pgErr.Hint)
	}

	if ok && pgErr.Source != nil {
		errCtx := pgErr.Source
		if errCtx.File != "" {
			c.writeBuf.putErrFieldMsg(pgwirebase.ServerErrFieldSrcFile)
			c.writeBuf.writeTerminatedString(errCtx.File)
		}

		if errCtx.Line > 0 {
			c.writeBuf.putErrFieldMsg(pgwirebase.ServerErrFieldSrcLine)
			c.writeBuf.writeTerminatedString(strconv.Itoa(int(errCtx.Line)))
		}

		if errCtx.Function != "" {
			c.writeBuf.putErrFieldMsg(pgwirebase.ServerErrFieldSrcFunction)
			c.writeBuf.writeTerminatedString(errCtx.Function)
		}
	}

	c.writeBuf.putErrFieldMsg(pgwirebase.ServerErrFieldMsgPrimary)
	c.writeBuf.writeTerminatedString(err.Error())

	c.writeBuf.nullTerminate()
	if err := c.writeBuf.finishMsg(c.wr); err != nil {
		return err
	}
	return c.wr.Flush()
}

// sendNoData sends NoData message when there aren't any rows to
// send. This must be set to true iff we are responding in the
// Extended Query protocol and the portal or statement will not return
// rows. See the notes about the NoData message in the Extended Query
// section of the docs here:
// https://www.postgresql.org/docs/9.6/static/protocol-flow.html#PROTOCOL-FLOW-EXT-QUERY
func (c *v3Conn) sendNoData(w io.Writer) error {
	c.writeBuf.initMsg(pgwirebase.ServerMsgNoData)
	return c.writeBuf.finishMsg(w)
}

// BeginCopyIn is part of the pgwirebase.Conn interface.
func (c *v3Conn) BeginCopyIn(ctx context.Context, columns []pgwire.ResultColumn) error {
	c.writeBuf.initMsg(pgwirebase.ServerMsgCopyInResponse)
	c.writeBuf.writeByte(byte(pgwirebase.FormatText))
	c.writeBuf.putInt16(int16(len(columns)))
	for range columns {
		c.writeBuf.putInt16(int16(pgwirebase.FormatText))
	}
	if err := c.writeBuf.finishMsg(c.wr); err != nil {
		return sql.NewWireFailureError(err)
	}
	if err := c.wr.Flush(); err != nil {
		return sql.NewWireFailureError(err)
	}
	return nil
}

// NewResultsGroup is part of the ResultsWriter interface.
func (c *v3Conn) NewResultsGroup() pgwire.ResultsGroup {
	return NewResultsGroup(&c.streamingState, c)
}

// SetEmptyQuery is part of the ResultsWriter interface.
func (c *v3Conn) SetEmptyQuery() {
	c.streamingState.emptyQuery = true
}

func (c *v3Conn) setError(err error) error {
	if _, isWireFailure := err.(sql.WireFailureError); isWireFailure {
		return err
	}

	state := &c.streamingState
	if state.err != nil {
		return state.err
	}

	state.hasSentResults = true
	state.err = err
	state.buf.Truncate(state.txnStartIdx)
	if err := c.flush(true /* forceSend */); err != nil {
		return sql.NewWireFailureError(err)
	}
	if err := c.SendError(err); err != nil {
		return sql.NewWireFailureError(err)
	}
	return nil
}

func (c *v3Conn) done() error {
	if err := c.flush(true /* forceSend */); err != nil {
		return err
	}

	state := &c.streamingState
	if state.err != nil {
		return nil
	}

	var err error
	if state.emptyQuery {
		// Generally a commandComplete message is written by each statement as it
		// finishes writing its results. Except in this emptyQuery case, where the
		// protocol mandates a particular response.
		c.writeBuf.initMsg(pgwirebase.ServerMsgEmptyQuery)
		err = c.writeBuf.finishMsg(c.wr)
	}

	if err != nil {
		return sql.NewWireFailureError(err)
	}
	return nil
}

// flush writes the streaming buffer to the underlying connection. If force
// is true then we will write any data we have buffered, otherwise we will
// only write when we exceed our buffer size.
func (c *v3Conn) flush(forceSend bool) error {
	state := &c.streamingState
	if state.buf.Len() == 0 {
		return nil
	}

	if forceSend || state.buf.Len() > connResultsBufferSizeBytes {
		state.hasSentResults = true
		state.txnStartIdx = 0
		if _, err := state.buf.WriteTo(c.wr); err != nil {
			return sql.NewWireFailureError(err)
		}
		if err := c.wr.Flush(); err != nil {
			return sql.NewWireFailureError(err)
		}
	}

	return nil
}

// Rd is part of the pgwirebase.Conn interface.
func (c *v3Conn) Rd() pgwirebase.BufferedReader {
	return &pgwireReader{conn: c}
}

// SendCommandComplete is part of the pgwirebase.Conn interface.
//func (c *v3Conn) SendCommandComplete(tag []byte) error {
//	return c.sendCommandComplete(tag, &c.streamingState.buf)
//}

// v3Conn implements pgwirebase.Conn.
var _ pgwirebase.Conn = &v3Conn{}

// pgwireReader is an io.Reader that wrapps a v3Conn, maintaining its metrics as
// it is consumed.
type pgwireReader struct {
	conn *v3Conn
}

// pgwireReader implements the pgwirebase.BufferedReader interface.
var _ pgwirebase.BufferedReader = &pgwireReader{}

// Read is part of the pgwirebase.BufferedReader interface.
func (r *pgwireReader) Read(p []byte) (int, error) {
	n, err := r.conn.rd.Read(p)
	return n, err
}

// ReadString is part of the pgwirebase.BufferedReader interface.
func (r *pgwireReader) ReadString(delim byte) (string, error) {
	s, err := r.conn.rd.ReadString(delim)
	return s, err
}

// ReadByte is part of the pgwirebase.BufferedReader interface.
func (r *pgwireReader) ReadByte() (byte, error) {
	b, err := r.conn.rd.ReadByte()
	return b, err
}

func newAdminShutdownErr(err error) error {
	return pgerror.NewErrorf(pgerror.CodeAdminShutdownError, err.Error())
}
