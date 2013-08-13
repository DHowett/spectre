package main

import (
	"net/http"
	"net/url"
)

type Model interface{}
type RenderContext struct {
	Obj     interface{}
	Request *http.Request
}

type HTTPError interface {
	StatusCode() int
}

type DeferLookupError struct {
	Interstitial *url.URL
}

func (d DeferLookupError) Error() string {
	return ""
}

type ModelRenderFunc func(Model, http.ResponseWriter, *http.Request)
type ModelLookupFunc func(*http.Request) (Model, error)

func RenderError(e error, statusCode int, w http.ResponseWriter) {
	w.WriteHeader(statusCode)
	ExecuteTemplate(w, "page_error", &RenderContext{e, nil})
}

func errorRecoveryHandler(w http.ResponseWriter) {
	if err := recover(); err != nil {
		status := http.StatusInternalServerError
		if weberr, ok := err.(HTTPError); ok {
			status = weberr.StatusCode()
		}

		RenderError(err.(error), status, w)
	}
}

// renderModelWith takes a template name and
// returns a function that takes a single model object,
// which when called will render the given template using that object.
func RenderTemplateForModel(template string) ModelRenderFunc {
	// We don't defer the error handler here because it happened a step up
	return func(o Model, w http.ResponseWriter, r *http.Request) {
		ExecuteTemplate(w, "page_"+template, &RenderContext{o, r})
	}
}

func RenderTemplateHandler(template string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer errorRecoveryHandler(w)
		ExecuteTemplate(w, "page_"+template, &RenderContext{nil, r})
	})
}

func RequiredModelObjectHandler(lookup ModelLookupFunc, fn ModelRenderFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer errorRecoveryHandler(w)

		if obj, err := lookup(r); err != nil {
			if dle, ok := err.(DeferLookupError); ok {
				http.SetCookie(w, &http.Cookie{
					Name:  "destination",
					Value: r.URL.String(),
					Path:  "/",
				})
				w.Header().Set("Location", dle.Interstitial.String())
				w.WriteHeader(http.StatusFound)
			} else {
				panic(err)
			}
		} else {
			fn(obj, w, r)
		}
	})
}
