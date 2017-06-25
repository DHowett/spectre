package main

import (
	"flag"

	"howett.net/spectre/http"
	"howett.net/spectre/postgres"
)

func main() {
	db := flag.String("db", "postgresql://ghostbin:password@localhost/ghostbintest?sslmode=disable", "database")
	flag.Parse()

	pconn, err := postgres.Open(*db)
	if err != nil {
		panic(err)
	}

	server := &http.Server{
		Addr:         ":8080",
		Proxied:      false,
		DocumentRoot: ".",

		PasteService: pconn.PasteService(),
	}
	server.Listen()
}
