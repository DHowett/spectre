package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"net/url"

	"github.com/golang/glog"
)

type Model interface{}

type CustomTemplateError interface {
	ErrorTemplateName() string
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
	page := "error"
	if cte, ok := e.(CustomTemplateError); ok {
		page = cte.ErrorTemplateName()
	}
	RenderPage(w, nil, page, e)
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
func RenderPageForModel(page string) ModelRenderFunc {
	// We don't defer the error handler here because it happened a step up
	return func(o Model, w http.ResponseWriter, r *http.Request) {
		RenderPage(w, r, page, o)
	}
}

func RenderPageHandler(page string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer errorRecoveryHandler(w)
		RenderPage(w, r, page, nil)
	})
}

func RenderPage(w io.Writer, r *http.Request, page string, obj interface{}) {
	ExecuteTemplate(w, "tmpl_page", &RenderContext{Request: r, Page: page, Obj: obj})
}

func RenderPartialHandler(page string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				RenderPartial(w, r, "error", err.(error))
			}
		}()
		RenderPartial(w, r, page, nil)
	})
}

func RenderPartial(w io.Writer, r *http.Request, name string, obj interface{}) {
	ExecuteTemplate(w, "partial_"+name, &RenderContext{Request: r, Page: name, Obj: obj})
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

func SetFlash(w http.ResponseWriter, kind, body string) {
	flashBody, err := json.Marshal(map[string]string{
		"type": kind,
		"body": body,
	})
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "flash",
		Value:  base64.URLEncoding.EncodeToString(flashBody),
		Path:   "/",
		MaxAge: 60,
	})
}

var ghosts []string

func init() {
	RegisterReloadFunction(func() {
		ghosts = []string{}
		err := YAMLUnmarshalFile("ghosts.yml", &ghosts)
		if err != nil {
			glog.Error(err)
		}
		for i, v := range ghosts {
			ghosts[i] = " " + v[1:]
		}
	})
	RegisterTemplateFunction("randomGhost", func() string {
		if len(ghosts) == 0 {
			return "[no ghosts found :(]"
		}
		return ghosts[rand.Intn(len(ghosts))]
	})
}
