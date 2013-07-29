package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"io"
	"launchpad.net/goyaml"
	"os"
)

type ReadCloser struct {
	io.Reader
	io.Closer
}

type WriteCloser struct {
	io.Writer
	io.Closer
}

func constructMAC(message, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	return mac.Sum(nil)
}

func checkMAC(message, messageMAC, key []byte) bool {
	return hmac.Equal(messageMAC, constructMAC(message, key))
}

func YAMLUnmarshalFile(filename string, i interface{}) error {
	yamlFile, err := os.Open(filename)
	if err != nil {
		return err
	}

	fi, err := yamlFile.Stat()
	if err != nil {
		return err
	}

	yml := make([]byte, fi.Size())
	io.ReadFull(yamlFile, yml)
	yamlFile.Close()
	err = goyaml.Unmarshal(yml, i)
	if err != nil {
		return err
	}

	return nil
}
