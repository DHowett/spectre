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

type _devZero struct{}

func (z *_devZero) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}

func (z *_devZero) Close() error {
	return nil
}

var devZero = &_devZero{}
