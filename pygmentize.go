package main

import (
	"bytes"
	"os/exec"
	"strings"
)

const pygmentizePath string = "./pygments/pygmentize"

func execWithData(data *string, cmd string, args ...string) (output string, err error) {
	var outbuf, errbuf bytes.Buffer
	pygments := exec.Command(cmd, args...)
	pygments.Stdin = strings.NewReader(*data)
	pygments.Stdout = &outbuf
	pygments.Stderr = &errbuf
	err = pygments.Run()
	output = strings.TrimSpace(outbuf.String())
	if(err != nil) {
		output = strings.TrimSpace(errbuf.String())
	}
	return
}

func PygmentsGuessLexer(data *string) (string, error) {
	return execWithData(data, pygmentizePath, "-G")
}

func Pygmentize(data *string, lexer string) (string, error) {
	return execWithData(data, pygmentizePath, "-f", "html", "-l", lexer, "-O", "nowrap=True,encoding=utf-8")
}
