package postgres

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"testing"

	"howett.net/spectre"

	"github.com/Sirupsen/logrus"
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

var pqPasteService spectre.PasteService
var pqUserService spectre.UserService
var pqGrantService spectre.GrantService
var pqReportService spectre.ReportService

func TestMain(m *testing.M) {
	db := flag.String("db", "postgresql://ghostbin:password@localhost/ghostbintest?sslmode=disable", "database")
	flag.Parse()
	sqlDb, err := sql.Open("postgres", *db)
	if err != nil {
		panic(err)
	}

	_, err = sqlDb.Exec(`
	DROP SCHEMA public CASCADE;
	CREATE SCHEMA public;
	GRANT ALL ON SCHEMA public TO postgres;
	GRANT ALL ON SCHEMA public TO ghostbin;
	GRANT ALL ON SCHEMA public TO public;
	`)
	fmt.Println(err)

	logger := logrus.New()
	logger.Formatter = &logrus.TextFormatter{
		ForceColors: true,
	}
	if testing.Verbose() {
		logger.Level = logrus.DebugLevel
	}
	prov, err := Open(*db)
	if err != nil {
		panic(err)
	}

	pqPasteService = prov.PasteService()
	pqUserService = prov.UserService()
	pqGrantService = prov.GrantService()
	pqReportService = prov.ReportService()

	e := m.Run()
	os.Exit(e)
}
