package querybuilder

import "testing"

func TestSqlite3(t *testing.T) {
	qb := New("sqlite3")

	upsert := UpsertQuery{
		Table:        "people",
		ConflictKeys: []string{"id"},
		Fields:       []string{"id", "name"},
	}

	t.Log(qb.Build(upsert))
}

func TestPostgres(t *testing.T) {
	qb := New("postgres")

	upsert := UpsertQuery{
		Table:        "people",
		ConflictKeys: []string{"id"},
		Fields:       []string{"id", "name"},
	}

	t.Log(qb.Build(upsert))
}
