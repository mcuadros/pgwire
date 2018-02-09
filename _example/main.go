package main

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/mcuadros/pgwire"
	"github.com/mcuadros/pgwire/server"
	"gopkg.in/sqle/sqle.v0/sql"

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
	sr.BeginResult(pgwire.Rows, pgwire.SELECT)
	sr.SetColumns(sql.Schema{
		{Name: "filename", Type: sql.String},
		{Name: "mode", Type: sql.String},
		{Name: "size", Type: sql.Integer},
		{Name: "mod_time", Type: sql.Timestamp},
		{Name: "is_dir", Type: sql.Boolean},
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
		sr.AddRow(s.Ctx(), []pgwire.Datum{
			pgwire.NewDString(file.Name()),
			pgwire.NewDString(file.Mode().String()),
			pgwire.NewDInt(file.Size()),
			pgwire.NewDTimestamp(file.ModTime(), time.Second),
			pgwire.NewDBool(file.IsDir()),
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
