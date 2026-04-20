package wiring

import (
	"fmt"
	"log/slog"
	"net/http"

	controlplanehttp "github.com/zaneway/AutoCertX/internal/app/controlplane/http"
	"github.com/zaneway/AutoCertX/internal/platform/buildinfo"
	"github.com/zaneway/AutoCertX/internal/platform/config"
	"github.com/zaneway/AutoCertX/internal/platform/logging"
	"github.com/zaneway/AutoCertX/internal/platform/runtime"
)

// Options controls dependency construction for the control plane process.
type Options struct {
	ServiceName     string
	EnvPrefix       string
	DefaultHTTPAddr string
}

// Result is the assembled process dependency graph for the control plane.
type Result struct {
	Config config.Config
	Logger *slog.Logger
	Server *http.Server
}

// Build assembles configuration, logger, HTTP router and HTTP server.
func Build(opts Options) (Result, error) {
	cfg, err := config.Load(config.LoadOptions{
		ServiceName:     opts.ServiceName,
		EnvPrefix:       opts.EnvPrefix,
		DefaultHTTPAddr: opts.DefaultHTTPAddr,
	})
	if err != nil {
		return Result{}, fmt.Errorf("load config: %w", err)
	}

	logger, err := logging.New(cfg.LogLevel)
	if err != nil {
		return Result{}, fmt.Errorf("new logger: %w", err)
	}

	build := buildinfo.Current(cfg.ServiceName)
	handler := controlplanehttp.NewRouter(controlplanehttp.Deps{
		Config:    cfg,
		BuildInfo: build,
		Logger:    logger,
	})

	return Result{
		Config: cfg,
		Logger: logger,
		Server: runtime.NewServer(cfg, handler),
	}, nil
}
