package main

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/mcuadros/pgwire"
	"github.com/mcuadros/pgwire/basesql"
	"github.com/mcuadros/pgwire/datum"
	"github.com/mcuadros/pgwire/server"
	"github.com/mcuadros/pgwire/types"
	"github.com/xwb1989/sqlparser"
)

func main() {
	cfg := server.NewConfig()
	cfg.Insecure = true

	s := server.New(cfg, NewExecutor())
	s.Start()
}

func NewExecutor() pgwire.Executor {
	return &executor{}
}

type executor struct{}

func (e *executor) ExecuteStatements(s pgwire.Session, rw pgwire.ResultsWriter, stmts string) error {
	fmt.Println("QUERY", stmts)

	stmt, err := sqlparser.Parse(stmts)
	if err != nil {
		return err
	}

	sel, ok := stmt.(*sqlparser.Select)
	if !ok {
		return fmt.Errorf("only SELECT operations are supported.")
	}

	group := rw.NewResultsGroup()

	defer group.Close()
	sr := group.NewStatementResult()
	sr.BeginResult(basesql.Rows, basesql.SELECT)
	sr.SetColumns(pgwire.ResultColumns{
		{Name: "filename", Typ: types.String},
		{Name: "mode", Typ: types.String},
		{Name: "size", Typ: types.Int},
		{Name: "mod_time", Typ: types.Timestamp},
		{Name: "is_dir", Typ: types.Bool},
	})

	for _, path := range getTableNames(sel) {
		if err := readDirectory(s, sr, path); err != nil {
			return err
		}
	}

	sr.CloseResult()
	group.Close()

	return nil
}

func readDirectory(s pgwire.Session, sr pgwire.StatementResult, path string) error {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}

	for _, file := range files {
		sr.AddRow(s.Ctx(), []datum.Datum{
			datum.NewDString(file.Name()),
			datum.NewDString(file.Mode().String()),
			datum.NewDInt(file.Size()),
			datum.NewDTimestamp(file.ModTime(), time.Second),
			datum.NewDBool(file.IsDir()),
		})
	}

	return nil
}

func (e *executor) RecordError(err error) {
	return
}

func getTableNames(s *sqlparser.Select) []string {
	var names []string

	for _, table := range s.From {
		alias, ok := table.(*sqlparser.AliasedTableExpr)
		if !ok {
			continue
		}

		names = append(names, sqlparser.GetTableName(alias.Expr).String())
	}

	return names
}
