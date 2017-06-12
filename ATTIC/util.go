package main

import (
	"crypto/rand"
	"encoding/base32"
	"net/http"
	"net/url"
	"strings"

	"github.com/DHowett/ghostbin/views"
	"github.com/gorilla/mux"
)

var base32Encoder = base32.NewEncoding("abcdefghjkmnopqrstuvwxyz23456789")

func generateRandomBytes(nbytes int) ([]byte, error) {
	uuid := make([]byte, nbytes)
	n, err := rand.Read(uuid)
	if n != len(uuid) || err != nil {
		return []byte{}, err
	}

	return uuid, nil
}

func generateRandomBase32String(outlen int) (string, error) {
	nbytes := (outlen * 5 / 8) + 1
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
	proto := strings.ToLower(r.URL.Scheme)
	return proto == "https"
}

func SourceIPForRequest(r *http.Request) string {
	ip := r.RemoteAddr[:strings.LastIndex(r.RemoteAddr, ":")]
	return ip
}

func HTTPSMuxMatcher(r *http.Request, rm *mux.RouteMatch) bool {
	return RequestIsHTTPS(r)
}

func NonHTTPSMuxMatcher(r *http.Request, rm *mux.RouteMatch) bool {
	return !RequestIsHTTPS(r)
}

func bindViews(viewModel *views.Model, dataProvider views.DataProvider, bmap map[interface{}]**views.View) error {
	var err error
	for id, vp := range bmap {
		*vp, err = viewModel.Bind(id, dataProvider)
		if err != nil {
			return err
		}
	}
	return nil
}
