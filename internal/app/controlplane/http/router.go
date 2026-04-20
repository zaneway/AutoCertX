package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/zaneway/AutoCertX/internal/app/controlplane/middleware"
	authcommand "github.com/zaneway/AutoCertX/internal/application/command/auth"
	caaccountscmd "github.com/zaneway/AutoCertX/internal/application/command/caaccounts"
	domainscmd "github.com/zaneway/AutoCertX/internal/application/command/domains"
	authcontextquery "github.com/zaneway/AutoCertX/internal/application/query/authcontext"
	"github.com/zaneway/AutoCertX/internal/platform/buildinfo"
	"github.com/zaneway/AutoCertX/internal/platform/config"
	"github.com/zaneway/AutoCertX/internal/platform/httpx"
)

// Deps contains the dependencies required to build the control plane router.
type Deps struct {
	Config            config.Config
	BuildInfo         buildinfo.Info
	Logger            *slog.Logger
	AuthService       *authcommand.Service
	AuthContextQuery  *authcontextquery.Service
	DomainCommands    *domainscmd.Service
	CAAccountCommands *caaccountscmd.Service
}

type healthResponse struct {
	Service     string `json:"service"`
	Environment string `json:"environment"`
	Status      string `json:"status"`
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	BuildTime   string `json:"buildTime"`
	Timestamp   string `json:"timestamp"`
}

// NewRouter builds the control plane HTTP handler tree.
func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		writeHealth(w, deps)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeHealth(w, deps)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		writeHealth(w, deps)
	})
	if deps.AuthService != nil && deps.AuthContextQuery != nil {
		registerAuthRoutes(mux, authHandler{
			authService:        deps.AuthService,
			authContextService: deps.AuthContextQuery,
		})
	}
	registerGovernanceRoutes(mux, deps)

	return middleware.Chain(
		mux,
		middleware.RequestID(),
		middleware.Recover(deps.Logger),
		middleware.AccessLog(deps.Logger),
	)
}

func writeHealth(w http.ResponseWriter, deps Deps) {
	payload := healthResponse{
		Service:     deps.Config.ServiceName,
		Environment: deps.Config.Environment,
		Status:      "ok",
		Version:     deps.BuildInfo.Version,
		Commit:      deps.BuildInfo.Commit,
		BuildTime:   deps.BuildInfo.BuildTime,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	_ = httpx.WriteJSON(w, http.StatusOK, payload)
}
