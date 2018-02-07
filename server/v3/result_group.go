package v3

import (
	"context"

	"github.com/mcuadros/pgwire"
)

type resultGroup struct {
	state *streamingState
	conn  *v3Conn
}

func NewResultsGroup(s *streamingState, conn *v3Conn) pgwire.ResultsGroup {
	return &resultGroup{state: s, conn: conn}
}

func (r *resultGroup) NewStatementResult() pgwire.StatementResult {
	return NewStatementResult(r.state, r.conn)
}

// ResultsSentToClient implements the ResultsGroup interface.
func (r *resultGroup) ResultsSentToClient() bool {
	return r.state.hasSentResults
}

// Close implements the ResultsGroup interface.
func (r *resultGroup) Close() {
	r.state.emptyQuery = false
	r.state.hasSentResults = false
}

// Reset implements the ResultsGroup interface.
func (r *resultGroup) Reset(ctx context.Context) {
	if r.state.hasSentResults {
		panic("cannot reset if we've already sent results for group")
	}

	r.state.emptyQuery = false
	r.state.buf.Truncate(0)
}

// Flush implements the ResultsGroup interface.
func (r *resultGroup) Flush(ctx context.Context) error {
	if err := r.conn.flush(true /* forceSend */); err != nil {
		return err
	}
	// hasSentResults is relative to the Flush() point, so we reset it here.
	r.state.hasSentResults = false
	return nil
}
