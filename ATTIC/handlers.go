package main

import (
	"net/http"
	"time"

	"github.com/DHowett/ghostbin/lib/rayman"
	"github.com/Sirupsen/logrus"
)

type RedirectHandler string

func (h RedirectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Location", string(h))
	w.WriteHeader(http.StatusFound)
}

func rayTimingHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := time.Now()
		h.ServeHTTP(w, r)
		t2 := time.Now()
		dur := t2.Sub(t)
		rayman.RequestLogger(r).WithFields(logrus.Fields{
			"facility": "http",
			"duration": int64(dur),
		}).Debugf("handler took %v", dur)
	})
}
