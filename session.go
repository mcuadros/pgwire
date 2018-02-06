package pgwire

import (
	"context"
	"time"

	"github.com/cockroachdb/cockroach/pkg/sql"
)

type Session interface {
	User() string
	Location() *time.Location
	Finish()
	FinishPlan()
	Ctx() context.Context
	Args() sql.SessionArgs
}

func NewSession(ctx context.Context, args sql.SessionArgs) Session {
	return &session{ctx: ctx, args: args}
}

type session struct {
	ctx  context.Context
	rw   ResultsWriter
	args sql.SessionArgs
}

func (s *session) User() string {
	return "foo bar"
}

func (s *session) Location() *time.Location {
	return nil
}

func (s *session) Ctx() context.Context {
	return context.Background()
}

func (s *session) Args() sql.SessionArgs {
	return s.args
}

func (s *session) SetArgs(args sql.SessionArgs) {
	s.args = args
}

func (s *session) Finish()     {}
func (s *session) FinishPlan() {}

type Executor interface {
	ExecuteStatements(s Session, rw ResultsWriter, stmts string) error
	RecordError(err error)
}
