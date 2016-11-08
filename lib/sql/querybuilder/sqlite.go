package querybuilder

import (
	"errors"
	"fmt"
	"strings"
)

type sqliteQueryBuilder struct{}

func (b *sqliteQueryBuilder) buildUpsert(qt *UpsertQuery) (string, error) {
	values := make([]string, len(qt.Fields))
	for i, _ := range qt.Fields {
		values[i] = "?"
	}
	return fmt.Sprintf("INSERT OR REPLACE INTO %s(%s) VALUES(%s)", qt.Table, strings.Join(qt.Fields, ","), strings.Join(values, ",")), nil
}

func (b *sqliteQueryBuilder) Build(q Query) (string, error) {

	switch q := q.(type) {
	case *UpsertQuery:
		return b.buildUpsert(q)
	case UpsertQuery:
		return b.buildUpsert(&q)
	default:
		return "", errors.New("querybuilder: I don't know what that query is.")
	}
}
