package runtime

import (
	"context"
	"fmt"

	"github.com/zaneway/AutoCertX/internal/platform/buildinfo"
	"github.com/zaneway/AutoCertX/internal/platform/config"
	"github.com/zaneway/AutoCertX/internal/platform/logging"
)

type Options struct {
	ServiceName     string
	EnvPrefix       string
	DefaultHTTPAddr string
}

func Run(ctx context.Context, opts Options) error {
	cfg, err := config.Load(config.LoadOptions{
		ServiceName:     opts.ServiceName,
		EnvPrefix:       opts.EnvPrefix,
		DefaultHTTPAddr: opts.DefaultHTTPAddr,
	})
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger, err := logging.New(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("new logger: %w", err)
	}

	build := buildinfo.Current(cfg.ServiceName)
	server := NewServer(cfg, NewHandler(cfg, build, logger))

	logger.Info("service starting",
		"service", cfg.ServiceName,
		"env", cfg.Environment,
		"httpAddr", cfg.HTTP.Addr,
	)

	return Serve(ctx, cfg.ServiceName, cfg.HTTP.ShutdownTimeout, logger, server)
}
