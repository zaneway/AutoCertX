package middleware

import "net/http"

// Middleware wraps an HTTP handler with cross-cutting behavior.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware in declaration order.
func Chain(next http.Handler, middlewares ...Middleware) http.Handler {
	wrapped := next
	for i := len(middlewares) - 1; i >= 0; i-- {
		wrapped = middlewares[i](wrapped)
	}

	return wrapped
}
