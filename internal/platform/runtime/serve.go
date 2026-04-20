package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

// Serve starts the HTTP server and manages graceful shutdown.
func Serve(ctx context.Context, serviceName string, shutdownTimeout time.Duration, logger *slog.Logger, server *http.Server) error {
	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stopCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		return fmt.Errorf("listen and serve: %w", err)
	case <-stopCtx.Done():
		logger.Info("service stopping", "service", serviceName)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}

	return nil
}
