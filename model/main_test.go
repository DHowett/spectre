package model

import (
	"database/sql"
	"flag"
	"os"
	"testing"

	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

type noopChallengeProvider struct{}

func (n *noopChallengeProvider) DeriveKey(string, []byte) []byte {
	return []byte{'a'}
}

func (n *noopChallengeProvider) RandomSalt() []byte {
	return []byte{'b'}
}

func (n *noopChallengeProvider) Challenge(message []byte, key []byte) []byte {
	return append(message, key...)
}

var broker Broker

func TestMain(m *testing.M) {
	flag.Parse()
	sqlDb, _ := sql.Open("sqlite3", ":memory:")
	broker, _ = NewDatabaseBroker("sqlite3", sqlDb, &noopChallengeProvider{})
	e := m.Run()
	os.Exit(e)
}
