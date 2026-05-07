package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/zaneway/AutoCertX/internal/app/controlplane/middleware"
	authcommand "github.com/zaneway/AutoCertX/internal/application/command/auth"
	caaccountscmd "github.com/zaneway/AutoCertX/internal/application/command/caaccounts"
	certificateassetscmd "github.com/zaneway/AutoCertX/internal/application/command/certificateassets"
	domainscmd "github.com/zaneway/AutoCertX/internal/application/command/domains"
	nodescmd "github.com/zaneway/AutoCertX/internal/application/command/nodes"
	settingscmd "github.com/zaneway/AutoCertX/internal/application/command/settings"
	targetscmd "github.com/zaneway/AutoCertX/internal/application/command/targets"
	auditquery "github.com/zaneway/AutoCertX/internal/application/query/audit"
	authcontextquery "github.com/zaneway/AutoCertX/internal/application/query/authcontext"
	dashboardquery "github.com/zaneway/AutoCertX/internal/application/query/dashboard"
	domainsquery "github.com/zaneway/AutoCertX/internal/application/query/domains"
	jobsquery "github.com/zaneway/AutoCertX/internal/application/query/jobs"
	settingsquery "github.com/zaneway/AutoCertX/internal/application/query/settings"
	deploymentservice "github.com/zaneway/AutoCertX/internal/deployment"
	"github.com/zaneway/AutoCertX/internal/driver/agenttransport"
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
	AgentTransport    *agenttransport.Service
	DeploymentService *deploymentservice.Service
	CertificateAssets *certificateassetscmd.Service
	DomainCommands    *domainscmd.Service
	CAAccountCommands *caaccountscmd.Service
	NodeCommands      *nodescmd.Service
	TargetCommands    *targetscmd.Service
	SettingsCommands  *settingscmd.Service
	GovernanceQuery   *domainsquery.Service
	SettingsQuery     *settingsquery.Service
	JobsQuery         *jobsquery.Service
	DashboardQuery    *dashboardquery.Service
	AuditQuery        *auditquery.Service
}

// healthResponse is the control-plane health/readiness payload.
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
		// Auth routes are only mounted when identity dependencies are wired so
		// control-plane startup can degrade cleanly in focused tests.
		registerAuthRoutes(mux, authHandler{
			authService:        deps.AuthService,
			authContextService: deps.AuthContextQuery,
		})
	}
	registerGovernanceRoutes(mux, deps)
	registerCertificateAssetRoutes(mux, deps)
	registerDeliveryRoutes(mux, deps)
	registerAgentTransportRoutes(mux, deps)
	registerAuditSettingsRoutes(mux, deps)
	registerRuntimeQueryRoutes(mux, deps)

	// The middleware order guarantees every downstream log/error payload sees the
	// request ID and recovery wraps the full handler tree.
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
