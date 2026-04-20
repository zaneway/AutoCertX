package bootstrap

import (
	"context"

	"github.com/zaneway/AutoCertX/internal/app/controlplane/wiring"
	"github.com/zaneway/AutoCertX/internal/platform/runtime"
)

// Options controls control plane bootstrap behavior.
type Options struct {
	ServiceName     string
	EnvPrefix       string
	DefaultHTTPAddr string
}

// Run wires and starts the control plane process.
func Run(ctx context.Context, opts Options) error {
	result, err := wiring.Build(wiring.Options{
		ServiceName:     opts.ServiceName,
		EnvPrefix:       opts.EnvPrefix,
		DefaultHTTPAddr: opts.DefaultHTTPAddr,
	})
	if err != nil {
		return err
	}

	result.Logger.Info("service starting",
		"service", result.Config.ServiceName,
		"env", result.Config.Environment,
		"httpAddr", result.Config.HTTP.Addr,
	)

	return runtime.Serve(ctx, result.Config.ServiceName, result.Config.HTTP.ShutdownTimeout, result.Logger, result.Server)
}
