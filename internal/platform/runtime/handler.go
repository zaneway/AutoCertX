package runtime

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/zaneway/AutoCertX/internal/platform/buildinfo"
	"github.com/zaneway/AutoCertX/internal/platform/config"
)

type healthResponse struct {
	Service     string `json:"service"`
	Environment string `json:"environment"`
	Status      string `json:"status"`
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	BuildTime   string `json:"buildTime"`
	Timestamp   string `json:"timestamp"`
}

func NewHandler(cfg config.Config, info buildinfo.Info, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		writeHealth(w, cfg, info)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeHealth(w, cfg, info)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		writeHealth(w, cfg, info)
	})

	return accessLog(logger, mux)
}

func writeHealth(w http.ResponseWriter, cfg config.Config, info buildinfo.Info) {
	payload := healthResponse{
		Service:     cfg.ServiceName,
		Environment: cfg.Environment,
		Status:      "ok",
		Version:     info.Version,
		Commit:      info.Commit,
		BuildTime:   info.BuildTime,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	writeJSON(w, http.StatusOK, payload)
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func accessLog(logger *slog.Logger, next http.Handler) http.Handler {
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

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}
