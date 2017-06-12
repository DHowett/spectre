package main

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

type BulkInserter struct {
	db *sqlx.DB

	// query as split by splitQuery
	begin, placeholders, end string
	stride, limit            int
	vals                     []interface{}

	// map [number of elements] -> query
	preparedStatements map[int]*sql.Stmt
}

// source: http://stackoverflow.com/questions/6581573/what-are-the-max-number-of-allowable-parameters-per-database-provider-type
func bindLimitForDriverName(driverName string) int {
	switch driverName {
	case "postgres":
		return 34464
	case "mysql":
		return 65535
	case "sqlite", "sqlite3":
		return 999 // SQLITE_MAX_VARIABLE_NUMBER defaults to 999
	}
	return -1
}

// splitQuery splits a query of the form INSERT INTO x(a, b, c) VALUES (?, ?, ?) into
// "INSERT INTO X(a, b, c) VALUES", "(?, ?, ?)" and ""
func splitQuery(query string) (string, string, string) {
	vi := strings.Index(query, "VALUES")
	if vi == -1 {
		return query, "", ""
	}
	open := vi + 6
	close := len(query)
	nparen := 0
	for i := open; i < len(query); i++ {
		if query[i] == '(' {
			nparen++
		} else if query[i] == ')' {
			nparen--
			if nparen == 0 {
				close = i
				break
			}
		}
	}
	return query[:open], query[open : close+1], query[close+1:]
}

func NewBulkInserter(db *sqlx.DB, query string) (*BulkInserter, error) {
	begin, placeholders, end := splitQuery(query)

	stride := strings.Count(placeholders, "?")
	if stride == 0 {
		return nil, fmt.Errorf("bulk: query `%s%s%s' doesn't contain any placeholders", begin, placeholders, end)
	}

	limit := bindLimitForDriverName(db.DriverName())

	if limit < 0 {
		return nil, fmt.Errorf("bulk: unable to prepare bulk inserts for sql.DB with driver `%s'", db.DriverName())
	}

	return &BulkInserter{
		db:           db,
		begin:        begin,
		placeholders: placeholders,
		stride:       stride,
		limit:        limit,
		end:          end,
		// optimistic assumption that we'll fill ~half the buffer
		vals:               make([]interface{}, 0, ((limit/stride)/2)*stride),
		preparedStatements: make(map[int]*sql.Stmt),
	}, nil
}

func (b *BulkInserter) preparedStmtForCount(n int) (*sql.Stmt, error) {
	stmt, ok := b.preparedStatements[n]
	if !ok {
		q := b.begin
		p := make([]string, n/b.stride)
		for i, _ := range p {
			p[i] = b.placeholders
		}
		q += strings.Join(p, ",")
		q += b.end
		var err error
		stmt, err = b.db.Prepare(b.db.Rebind(q))
		if err != nil {
			return nil, err
		}
		b.preparedStatements[n] = stmt
	}
	return stmt, nil
}

func (b *BulkInserter) exec(vals []interface{}) error {
	n := len(vals)
	if n%b.stride != 0 {
		return fmt.Errorf("bulk: asked to insert %d values with a stride of %d?", n, b.stride)
	}

	stmt, err := b.preparedStmtForCount(n)
	if err != nil {
		return err
	}

	_, err = stmt.Exec(vals...)

	if err != nil {
		// if we had an error...
		if n > b.stride {
			// ... and more than one record
			nr := n / b.stride
			nr = nr / 2
			mid := nr * b.stride
			errL, errR := b.exec(vals[:mid]), b.exec(vals[mid:])
			if errL != nil {
				if errR != nil {
					// both errors set
					err = fmt.Errorf("bulk: split exec got left and right errors %v and %v", errL, errR)
				} else {
					// only left set
					err = errL
				}
			} else {
				// only right set (or unset; doesn't matter)
				err = errR
			}
		} else {
			// ... and only one record
			err = fmt.Errorf("bulk: record (%v): %v", vals, err)
		}
	}

	return err
}

func (b *BulkInserter) Flush() error {
	if len(b.vals) == 0 {
		return nil
	}

	err := b.exec(b.vals)
	b.vals = b.vals[0:0]
	return err
}

func (b *BulkInserter) Insert(vals ...interface{}) error {
	var err error
	if len(b.vals)+len(vals) > b.limit {
		err = b.Flush()
	}
	b.vals = append(b.vals, vals...)
	return err
}
