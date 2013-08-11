package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"github.com/golang/glog"
	"io"
	"launchpad.net/goyaml"
	"os"
	"os/signal"
	"syscall"
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

var base32Encoder = base32.NewEncoding("abcdefghjkmnopqrstuvwxyz23456789")

func generateRandomBytes(nbytes int) ([]byte, error) {
	uuid := make([]byte, nbytes)
	n, err := rand.Read(uuid)
	if n != len(uuid) || err != nil {
		return []byte{}, err
	}

	return uuid, nil
}

func generateRandomBase32String(nbytes, outlen int) (string, error) {
	uuid, err := generateRandomBytes(nbytes)
	if err != nil {
		return "", err
	}

	s := base32Encoder.EncodeToString(uuid)
	if outlen == -1 {
		outlen = len(s)
	}

	return s[0:outlen], nil
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

type ReloadFunction func()

var reloadFunctions = []ReloadFunction{}

func RegisterReloadFunction(f ReloadFunction) {
	reloadFunctions = append(reloadFunctions, f)
}

func ReloadAll() {
	for _, f := range reloadFunctions {
		f()
	}
}

func init() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)
	go func() {
		for _ = range sigChan {
			glog.Info("Received SIGHUP")
			ReloadAll()
		}
	}()
}
