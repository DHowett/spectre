package main

import (
	"bytes"
	"os/exec"
	"strings"
)

const pygmentizePath string = "./pygments/pygmentize"

func execWithData(data *string, cmd string, args ...string) (string, error) {
	var outbuf bytes.Buffer
	pygments := exec.Command(cmd, args...)
	pygments.Stdin = strings.NewReader(*data)
	pygments.Stdout = &outbuf
	err := pygments.Run()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(outbuf.String()), nil
}

func PygmentsGuessLexer(data *string) (string, error) {
	return execWithData(data, pygmentizePath, "-G")
}

func Pygmentize(data *string) (string, error) {
	return execWithData(data, pygmentizePath, "-f", "html", "-O", "nowrap=True", "-g")
}
