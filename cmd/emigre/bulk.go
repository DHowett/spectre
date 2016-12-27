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

func NewBulkInserter(db *sqlx.DB, query string) *BulkInserter {
	begin, placeholders, end := splitQuery(query)
	return &BulkInserter{
		db:                 db,
		begin:              begin,
		placeholders:       placeholders,
		stride:             strings.Count(placeholders, "?"),
		limit:              30000, // psql allows 34000?
		end:                end,
		preparedStatements: make(map[int]*sql.Stmt),
	}
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
	if len(vals)%b.stride != 0 {
		return fmt.Errorf("bulk: asked to insert %d values with a stride of %d?", len(vals), b.stride)
	}

	stmt, err := b.preparedStmtForCount(len(vals))
	if err != nil {
		return err
	}

	_, err = stmt.Exec(vals...)

	if err != nil {
		// if we had an error...
		if len(vals) > b.stride {
			// ... and more than one record
			n := len(vals) / b.stride
			n = n / 2
			mid := n * b.stride
			errL, errR := b.exec(vals[:mid]), b.exec(vals[mid:])
			if errL != nil && errR != nil {
				return fmt.Errorf("bulk: split exec got left and right errors %v and %v", errL, errR)
			}
			if errL != nil {
				err = errL
			} else {
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
