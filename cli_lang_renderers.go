package main

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
)

func execWithStream(stream io.Reader, args ...string) (output string, err error) {
	var outbuf, errbuf bytes.Buffer
	command := exec.Command(args[0], args[1:]...)
	command.Stdin = stream
	command.Stdout = &outbuf
	command.Stderr = &errbuf
	err = command.Run()
	output = strings.TrimSpace(outbuf.String())
	if err != nil {
		output = strings.TrimSpace(errbuf.String())
	}
	return
}
