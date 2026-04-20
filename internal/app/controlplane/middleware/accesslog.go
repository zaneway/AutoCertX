package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// AccessLog records basic HTTP request information.
func AccessLog(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(recorder, r)

			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", recorder.statusCode,
				"duration", time.Since(start),
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}
