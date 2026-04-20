package runtime

import (
	"net/http"

	"github.com/zaneway/AutoCertX/internal/platform/config"
)

func NewServer(cfg config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           handler,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}
}
