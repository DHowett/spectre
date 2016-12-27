package main

import (
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

type BulkInserter struct {
	db *sqlx.DB

	// query as split by splitQuery
	begin, placeholders, end string
	count, limit             int
	values                   []interface{}

	// map [number of elements] -> query
	preparedStatements map[int]*sqlx.Stmt
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
		count:              strings.Count(placeholders, "?"),
		limit:              30000, // psql allows 34000?
		end:                end,
		preparedStatements: make(map[int]*sqlx.Stmt),
	}
}

func (b *BulkInserter) flush(vals []interface{}) error {
	if len(vals)%b.count != 0 {
		return fmt.Errorf("bulk: asked to insert %d values with a stride of %d?", len(vals), b.count)
	}

	query, ok := b.preparedStatements[len(vals)]
	if !ok {
		q := b.begin
		p := make([]string, len(vals)/b.count)
		for i, _ := range p {
			p[i] = b.placeholders
		}
		q += strings.Join(p, ",")
		q += b.end
		var err error
		query, err = b.db.Preparex(b.db.Rebind(q))
		if err != nil {
			return err
		}
		b.preparedStatements[len(vals)] = query
	}
	_, err := query.Exec(vals...)
	return err
}

func (b *BulkInserter) binarySplitFlush(vals []interface{}) error {
	if len(vals) <= b.count {
		// can't split any further.
		err := b.flush(vals)
		if err != nil {
			return fmt.Errorf("bulk: record (%v) error %v", vals, err)
		}
	}

	n := len(vals) / b.count
	n = n / 2
	mid := n * b.count

	errLeft := b.flush(vals[:mid])  // insert low half
	errRight := b.flush(vals[mid:]) // insert high half
	if errLeft != nil {
		// if the low half failed, partition further
		errLeft = b.binarySplitFlush(vals[:mid])
	}

	if errRight != nil {
		// if the high half failed, partition further
		errRight = b.binarySplitFlush(vals[mid:])
	}

	if errLeft != nil && errRight != nil {
		return fmt.Errorf("bulk: split flush got left and right errors %v and %v", errLeft, errRight)
	}
	if errLeft != nil {
		return errLeft
	}
	return errRight
}

func (b *BulkInserter) Flush() error {
	if len(b.values) == 0 {
		return nil
	}

	err := b.flush(b.values)
	if err != nil {
		// binary search insert; narrow down to the single broken value
		err = b.binarySplitFlush(b.values)
	}
	b.values = b.values[0:0]
	return err
}

func (b *BulkInserter) Insert(vals ...interface{}) error {
	var err error
	if len(b.values)+len(vals) > b.limit {
		err = b.Flush()
	}
	b.values = append(b.values, vals...)
	return err
}
