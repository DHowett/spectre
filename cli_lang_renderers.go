package main

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
)

const pygmentizePath string = "./pygments/pygmentize"

func execWithStream(stream io.Reader, cmd string, args ...string) (output string, err error) {
	var outbuf, errbuf bytes.Buffer
	pygments := exec.Command(cmd, args...)
	pygments.Stdin = stream
	pygments.Stdout = &outbuf
	pygments.Stderr = &errbuf
	err = pygments.Run()
	output = strings.TrimSpace(outbuf.String())
	if err != nil {
		output = strings.TrimSpace(errbuf.String())
	}
	return
}

func Pygmentize(stream io.Reader, lexer string) (string, error) {
	return execWithStream(stream, pygmentizePath, "-f", "html", "-l", lexer, "-O", "nowrap=True,encoding=utf-8")
}

func ANSI(stream io.Reader) (string, error) {
	return execWithStream(stream, "./ansi2html", "--naked")
}
