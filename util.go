package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"gopkg.in/yaml.v2"
)

const (
	EnvironmentDevelopment string = "dev"
	EnvironmentProduction  string = "production"

	SPECTRE_DEFAULT_BRAND string = "Spectre"
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
	err = yaml.Unmarshal(yml, i)
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

func BaseURLForRequest(r *http.Request) *url.URL {
	determinedScheme := "http"
	if RequestIsHTTPS(r) {
		determinedScheme = "https"
	}
	return &url.URL{
		Scheme: determinedScheme,
		User:   r.URL.User,
		Host:   r.Host,
		Path:   "/",
	}
}

func RequestIsHTTPS(r *http.Request) bool {
	proto := strings.ToLower(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		proto = strings.ToLower(r.URL.Scheme)
	}
	return proto == "https"
}

func SourceIPForRequest(r *http.Request) string {
	ip := r.Header.Get("CF-Connecting-IP")
	if ip == "" {
		ip := r.Header.Get("X-Forwarded-For")
		if ip == "" {
			ip = r.RemoteAddr[:strings.LastIndex(r.RemoteAddr, ":")]
		}
	}
	return ip
}

func HTTPSMuxMatcher(r *http.Request, rm *mux.RouteMatch) bool {
	return Env() == EnvironmentDevelopment || RequestIsHTTPS(r)
}

func NonHTTPSMuxMatcher(r *http.Request, rm *mux.RouteMatch) bool {
	return !HTTPSMuxMatcher(r, rm)
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

type ByteSize float64

const (
	_           = iota // ignore first value by assigning to blank identifier
	KB ByteSize = 1 << (10 * iota)
	MB
	GB
	TB
	PB
	EB
	ZB
	YB
)

func (b ByteSize) String() string {
	switch {
	case b >= YB:
		return fmt.Sprintf("%.2fYB", b/YB)
	case b >= ZB:
		return fmt.Sprintf("%.2fZB", b/ZB)
	case b >= EB:
		return fmt.Sprintf("%.2fEB", b/EB)
	case b >= PB:
		return fmt.Sprintf("%.2fPB", b/PB)
	case b >= TB:
		return fmt.Sprintf("%.2fTB", b/TB)
	case b >= GB:
		return fmt.Sprintf("%.2fGB", b/GB)
	case b >= MB:
		return fmt.Sprintf("%.2fMB", b/MB)
	case b >= KB:
		return fmt.Sprintf("%.2fKB", b/KB)
	}
	return fmt.Sprintf("%.2fB", b)
}

var environment string = EnvironmentDevelopment

func Env() string {
	return environment
}

func init() {
	environment = os.Getenv("SPECTRE_ENV")
	if environment != EnvironmentProduction {
		environment = EnvironmentDevelopment
	}

	brand := os.Getenv("SPECTRE_BRAND")
	if brand == "" {
		brand = SPECTRE_DEFAULT_BRAND
	}

	RegisterTemplateFunction("env", func() string { return environment })

	RegisterTemplateFunction("brand", func() string {
		return brand
	})

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)
	go func() {
		for _ = range sigChan {
			glog.Info("Received SIGHUP")
			ReloadAll()
		}
	}()
}
