package rayman

import (
	"net/http"

	"github.com/Sirupsen/logrus"
)

func RequestLogger(r *http.Request) logrus.FieldLogger {
	return ContextLogger(r.Context())
}

func LoggingHandler(h http.Handler, logger logrus.FieldLogger) http.Handler {
	return Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		rid, ok := FromContext(ctx)
		if ok {
			rayedLogger := logger.WithFields(logrus.Fields{
				"ray":  rid,
				"path": r.URL.Path,
			})
			r = r.WithContext(contextWithLogger(ctx, rayedLogger))
		}
		h.ServeHTTP(w, r)
	}))
}
