package main

import (
	"howett.net/spectre/http"
)

func main() {
	us := &mockUserService{}
	server := &http.Server{
		Addr:         ":8080",
		Proxied:      false,
		DocumentRoot: ".",

		PasteService:             &mockPasteService{},
		RequestPermitterProvider: loggingPermitter{},
		UserService:              us,
		LoginService:             &mockLoginService{UserService: us},
	}
	server.Listen()
}
