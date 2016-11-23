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
