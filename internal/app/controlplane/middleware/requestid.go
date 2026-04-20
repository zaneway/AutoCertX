package middleware

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

const requestIDHeader = "X-Request-Id"

var requestIDCounter atomic.Uint64

// RequestID attaches a stable request identifier to each response.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get(requestIDHeader)
			if requestID == "" {
				requestID = nextRequestID()
			}

			w.Header().Set(requestIDHeader, requestID)
			next.ServeHTTP(w, r)
		})
	}
}

func nextRequestID() string {
	return fmt.Sprintf("%d-%d", time.Now().UTC().UnixNano(), requestIDCounter.Add(1))
}
