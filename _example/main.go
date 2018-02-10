package main

import (
	"time"

	"gopkg.in/sqle/sqle.v0"
	"gopkg.in/sqle/sqle.v0/mem"
	"gopkg.in/sqle/sqle.v0/sql"

	"github.com/mcuadros/pgwire/server"
)

func main() {
	e := sqle.New()
	e.AddDatabase(createTestDatabase())

	cfg := server.NewConfig()
	cfg.Insecure = true

	s := server.New(cfg, e)
	s.Start()
}

func createTestDatabase() *mem.Database {
	db := mem.NewDatabase("test")
	table := mem.NewTable("mytable", sql.Schema{
		{Name: "name", Type: sql.String},
		{Name: "email", Type: sql.String},
		{Name: "created_at", Type: sql.TimestampWithTimezone},
	})
	db.AddTable("mytable", table)
	table.Insert(sql.NewRow("John Doe", "john@doe.com", time.Now()))
	table.Insert(sql.NewRow("John Doe", "johnalt@doe.com", time.Now()))
	table.Insert(sql.NewRow("Jane Doe", "jane@doe.com", time.Now()))
	table.Insert(sql.NewRow("Evil Bob", "evilbob@gmail.com", time.Now()))
	return db
}
