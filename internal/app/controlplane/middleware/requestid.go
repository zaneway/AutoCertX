package middleware

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

const requestIDHeader = "X-Request-Id"

var requestIDCounter atomic.Uint64

// requestIDContextKey stores the generated request ID in context.
type requestIDContextKey struct{}

// RequestID attaches a stable request identifier to each response.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get(requestIDHeader)
			if requestID == "" {
				// Fall back to a locally generated identifier so every request can be
				// traced even when the caller does not provide one.
				requestID = nextRequestID()
			}

			w.Header().Set(requestIDHeader, requestID)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDContextKey{}, requestID)))
		})
	}
}

// RequestIDFromContext returns the request ID injected by RequestID middleware.
func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func nextRequestID() string {
	return fmt.Sprintf("%d-%d", time.Now().UTC().UnixNano(), requestIDCounter.Add(1))
}
