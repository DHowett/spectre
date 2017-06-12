package querybuilder

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type postgresQueryBuilder struct{}

func (b *postgresQueryBuilder) escapeTable(table string) string {
	return `"` + table + `"`
}

func (b *postgresQueryBuilder) escapeField(field string) string {
	return `"` + field + `"`
}

func (b *postgresQueryBuilder) buildUpsert(q *UpsertQuery) (string, error) {
	escapedConflictKeys := make([]string, len(q.ConflictKeys))
	conflicts := map[string]struct{}{}
	for i, v := range q.ConflictKeys {
		escapedConflictKeys[i] = b.escapeField(v)
		conflicts[v] = struct{}{}
	}

	escapedFields := make([]string, len(q.Fields))
	values := make([]string, len(q.Fields))
	replacements := make([]string, 0, len(q.Fields))
	for i, v := range q.Fields {
		escapedFields[i] = b.escapeField(v)
		values[i] = "$" + strconv.Itoa(i+1)
		if _, present := conflicts[v]; !present {
			replacements = append(replacements, fmt.Sprintf(`%s = EXCLUDED.%s`, b.escapeField(v), b.escapeField(v)))
		}
	}
	return fmt.Sprintf("INSERT INTO %s(%s) VALUES(%s) ON CONFLICT(%s) DO UPDATE SET %s", b.escapeTable(q.Table), strings.Join(escapedFields, ","), strings.Join(values, ","), strings.Join(escapedConflictKeys, ","), strings.Join(replacements, ",")), nil
}

func (b *postgresQueryBuilder) Build(q Query) (string, error) {
	switch q := q.(type) {
	case *UpsertQuery:
		return b.buildUpsert(q)
	case UpsertQuery:
		return b.buildUpsert(&q)
	default:
		return "", errors.New("querybuilder: I don't know what that query is.")
	}
}
