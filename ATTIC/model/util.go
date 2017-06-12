package model

import "io"

type readCloser struct {
	io.Reader
	io.Closer
}

type writeCloser struct {
	io.Writer
	io.Closer
}
