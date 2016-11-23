package main

import "net/http"

type WebError interface {
	Error() string
	StatusCode() int
}

type webError struct {
	e string
	s int
}

func (e webError) Error() string {
	return e.e
}

func (e webError) StatusCode() int {
	return e.s
}

var (
	webErrThrottled        = webError{"Cool it.", 420}
	webErrEmptyPaste       = webError{"Hey, put some text in that paste!", http.StatusBadRequest}
	webErrInsecurePassword = webError{"I refuse to accept passwords over http.", http.StatusBadRequest}
)
