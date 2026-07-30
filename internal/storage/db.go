// Package storage.
package storage

import (
	"database/sql"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var schemas = []string{
	jobSchema,
	taskSchema,
	videoSchema,
	threadSchema,
	commentSchema,
	indexes,
}

func Init(path string) (*sql.DB, error) {
	schema := strings.Join(schemas, "\n")
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return db, nil
}

