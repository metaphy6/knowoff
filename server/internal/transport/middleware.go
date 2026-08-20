package transport

import (
	"log/slog"
	"net/http"
)

// RecoverPanic wraps a handler so that a panic in any HTTP handler is logged
// and returned as a 500 error without killing the process.
func RecoverPanic(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("handler panic recovered",
					"panic", rec,
					"path", r.URL.Path,
					"method", r.Method,
				)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// RequestID is a placeholder middleware that tags requests; extended in later
// phases to attach per-connection ids to logs.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
