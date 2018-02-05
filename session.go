package pgwire

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/cockroach/pkg/sql/sem/tree"
	"github.com/mcuadros/pgwire/datum"
	"github.com/mcuadros/pgwire/types"
)

type Session interface {
	User() string
	Location() *time.Location
	Finish()
	FinishPlan()
	Ctx() context.Context

	ResultsWriter() ResultsWriter
	SetResultsWriter(rw ResultsWriter)
}

func NewSession(ctx context.Context) Session {
	return &session{ctx: ctx}
}

type session struct {
	ctx context.Context
	rw  ResultsWriter
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

func (s *session) ResultsWriter() ResultsWriter {
	return s.rw
}

func (s *session) SetResultsWriter(rw ResultsWriter) {
	s.rw = rw
}

func (s *session) Finish()     {}
func (s *session) FinishPlan() {}

type Executor interface {
	ExecuteStatements(s Session, stmts string) error
	RecordError(err error)
}

func NewExecutor() Executor {
	return &executor{}
}

type executor struct{}

func (e *executor) ExecuteStatements(s Session, stmts string) error {
	fmt.Println("QUERY", stmts)
	sss := datum.DString("foo")
	group := s.ResultsWriter().NewResultsGroup()

	defer group.Close()
	sr := group.NewStatementResult()
	sr.BeginResult((*tree.Select)(nil))
	sr.SetColumns(ResultColumns{{Name: "test", Typ: types.String}})

	for i := 0; i < 10; i++ {
		if err := sr.AddRow(s.Ctx(), []datum.Datum{
			&sss,
		}); err != nil {
			panic(err)
		}
	}

	sr.CloseResult()
	group.Close()

	return nil
}

func (e *executor) RecordError(err error) {
	return
}
