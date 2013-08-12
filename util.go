package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"github.com/golang/glog"
	"io"
	"launchpad.net/goyaml"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

const (
	EnvironmentDevelopment string = "dev"
	EnvironmentProduction  string = "production"
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

func SlurpFile(path string) (out []byte, err error) {
	var file *os.File
	if file, err = os.Open(path); err == nil {
		buf := &bytes.Buffer{}
		io.Copy(buf, file)
		out = buf.Bytes()
		file.Close()
	}
	return
}

func RequestIsHTTPS(r *http.Request) bool {
	if Env() == EnvironmentDevelopment {
		return true
	}

	proto := strings.ToLower(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		proto = strings.ToLower(r.URL.Scheme)
	}
	return proto == "https"
}

func SourceIPForRequest(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr[:strings.LastIndex(r.RemoteAddr, ":")]
	}
	return ip
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

var environment string

func Env() string {
	return environment
}

func init() {
	environment = os.Getenv("GHOSTBIN_ENV")
	if environment != EnvironmentProduction {
		environment = EnvironmentDevelopment
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)
	go func() {
		for _ = range sigChan {
			glog.Info("Received SIGHUP")
			ReloadAll()
		}
	}()
}
