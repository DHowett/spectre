package main

import (
	"bytes"
	"os/exec"
	"strings"
)

func Pygmentize(in string) (string, error) {
	var outbuf bytes.Buffer
	pygments := exec.Command("/Users/dustin/junk/pygments/pygmentize", "-f", "html", "-O", "nowrap=True", "-g")
	pygments.Stdin = strings.NewReader(in)
	pygments.Stdout = &outbuf
	err := pygments.Run()
	if err != nil {
		return "", err
	}
	return outbuf.String(), nil
}
