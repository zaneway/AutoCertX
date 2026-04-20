package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/zaneway/AutoCertX/internal/app/controlplane/middleware"
	authcommand "github.com/zaneway/AutoCertX/internal/application/command/auth"
	authcontextquery "github.com/zaneway/AutoCertX/internal/application/query/authcontext"
	"github.com/zaneway/AutoCertX/internal/domain/identity"
	"github.com/zaneway/AutoCertX/internal/domain/tenancy"
	"github.com/zaneway/AutoCertX/internal/platform/httpx"
)

const (
	headerAuthorization = "Authorization"
	headerTenantID      = "X-Tenant-Id"
	headerProjectID     = "X-Project-Id"
	headerEnvironmentID = "X-Environment-Id"
)

type authHandler struct {
	authService        *authcommand.Service
	authContextService *authcontextquery.Service
}

type authLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authRefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type localePreferencesRequest struct {
	Locale string `json:"locale"`
}

type authLoginResponse struct {
	RequestID    string                `json:"request_id"`
	AccessToken  string                `json:"access_token"`
	RefreshToken string                `json:"refresh_token"`
	ExpiresIn    int                   `json:"expires_in"`
	Context      authcontextquery.View `json:"context"`
}

type authMeResponse struct {
	RequestID string                `json:"request_id"`
	Data      authcontextquery.View `json:"data"`
}

type emptyEnvelope struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
}

type errorResponse struct {
	RequestID string `json:"request_id"`
	Error     struct {
		Code    string        `json:"code"`
		Message string        `json:"message"`
		Details []errorDetail `json:"details,omitempty"`
	} `json:"error"`
}

type errorDetail struct {
	Field  string `json:"field,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type principalContextKey struct{}

func registerAuthRoutes(mux *http.ServeMux, handler authHandler) {
	mux.HandleFunc("/api/v1/auth/login", handler.handleLogin)
	mux.HandleFunc("/api/v1/auth/refresh", handler.handleRefresh)

	logout := handler.withAuthentication(http.HandlerFunc(handler.handleLogout))
	mux.Handle("/api/v1/auth/logout", logout)

	me := handler.withAuthentication(handler.withPermissions(
		http.HandlerFunc(handler.handleMe),
		tenancy.PermissionAuthContextRead,
	))
	mux.Handle("/api/v1/auth/me", me)

	preferences := handler.withAuthentication(handler.withPermissions(
		http.HandlerFunc(handler.handlePreferences),
		tenancy.PermissionAuthPreferencesWrite,
	))
	mux.Handle("/api/v1/auth/me/preferences", preferences)
}

func (h authHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	var req authLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}
	if strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.Password) == "" {
		writeValidationError(w, r, "username", "required")
		return
	}

	result, err := h.authService.Login(
		r.Context(),
		req.Username,
		req.Password,
		selectionFromHeaders(r.Header),
		sessionMetadataFromRequest(r),
	)
	if err != nil {
		writeMappedError(w, r, err)
		return
	}

	view := h.authContextService.Build(result.Principal.User, result.Principal.Context)
	_ = httpx.WriteJSON(w, http.StatusOK, authLoginResponse{
		RequestID:    requestID(r),
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    result.ExpiresIn,
		Context:      view,
	})
}

func (h authHandler) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	var req authRefreshRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}
	if strings.TrimSpace(req.RefreshToken) == "" {
		writeValidationError(w, r, "refresh_token", "required")
		return
	}

	result, err := h.authService.Refresh(
		r.Context(),
		req.RefreshToken,
		selectionFromHeaders(r.Header),
		sessionMetadataFromRequest(r),
	)
	if err != nil {
		writeMappedError(w, r, err)
		return
	}

	view := h.authContextService.Build(result.Principal.User, result.Principal.Context)
	_ = httpx.WriteJSON(w, http.StatusOK, authLoginResponse{
		RequestID:    requestID(r),
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    result.ExpiresIn,
		Context:      view,
	})
}

func (h authHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	principal, ok := principalFromRequest(r)
	if !ok {
		writeMappedError(w, r, identity.ErrUnauthorized)
		return
	}

	if err := h.authService.Logout(r.Context(), principal); err != nil {
		writeMappedError(w, r, err)
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, emptyEnvelope{
		RequestID: requestID(r),
		Status:    "ok",
	})
}

func (h authHandler) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	principal, ok := principalFromRequest(r)
	if !ok {
		writeMappedError(w, r, identity.ErrUnauthorized)
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, authMeResponse{
		RequestID: requestID(r),
		Data:      h.authContextService.Build(principal.User, principal.Context),
	})
}

func (h authHandler) handlePreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.NotFound(w, r)
		return
	}

	principal, ok := principalFromRequest(r)
	if !ok {
		writeMappedError(w, r, identity.ErrUnauthorized)
		return
	}

	var req localePreferencesRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}
	if !isSupportedLocale(req.Locale) {
		writeValidationError(w, r, "locale", "unsupported")
		return
	}

	updatedPrincipal, err := h.authService.UpdateLocale(r.Context(), principal, req.Locale)
	if err != nil {
		writeMappedError(w, r, err)
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, authMeResponse{
		RequestID: requestID(r),
		Data:      h.authContextService.Build(updatedPrincipal.User, updatedPrincipal.Context),
	})
}

func (h authHandler) withAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get(headerAuthorization))
		if token == "" {
			writeMappedError(w, r, identity.ErrUnauthorized)
			return
		}

		principal, err := h.authService.Authenticate(r.Context(), token, selectionFromHeaders(r.Header))
		if err != nil {
			writeMappedError(w, r, err)
			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalContextKey{}, principal)))
	})
}

func (h authHandler) withPermissions(next http.Handler, permissions ...tenancy.Permission) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromRequest(r)
		if !ok {
			writeMappedError(w, r, identity.ErrUnauthorized)
			return
		}

		for _, permission := range permissions {
			if !principal.Context.HasPermission(permission) {
				writeMappedError(w, r, tenancy.ErrPermissionDenied)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func principalFromRequest(r *http.Request) (authcommand.Principal, bool) {
	principal, ok := r.Context().Value(principalContextKey{}).(authcommand.Principal)
	return principal, ok
}

func selectionFromHeaders(header http.Header) authcommand.Selection {
	return authcommand.Selection{
		TenantID:      header.Get(headerTenantID),
		ProjectID:     header.Get(headerProjectID),
		EnvironmentID: header.Get(headerEnvironmentID),
	}
}

func sessionMetadataFromRequest(r *http.Request) identity.SessionMetadata {
	clientIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		clientIP = host
	}

	return identity.SessionMetadata{
		ClientIP:  clientIP,
		UserAgent: r.UserAgent(),
	}
}

func decodeJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func requestID(r *http.Request) string {
	if id := middleware.RequestIDFromContext(r.Context()); id != "" {
		return id
	}
	return r.Header.Get("X-Request-Id")
}

func bearerToken(value string) string {
	if !strings.HasPrefix(value, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
}

func isSupportedLocale(locale string) bool {
	return locale == authcommand.LocaleZH || locale == authcommand.LocaleEN
}

func writeMappedError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrInvalidCredentials):
		writeError(w, r, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "authentication failed", nil)
	case errors.Is(err, identity.ErrSessionExpired):
		writeError(w, r, http.StatusUnauthorized, "AUTH_SESSION_EXPIRED", "session expired", nil)
	case errors.Is(err, tenancy.ErrPermissionDenied):
		writeError(w, r, http.StatusForbidden, "PERMISSION_DENIED", "permission denied", nil)
	case errors.Is(err, tenancy.ErrScopeMismatch):
		writeError(w, r, http.StatusConflict, "TENANT_SCOPE_MISMATCH", "scope mismatch", nil)
	case errors.Is(err, identity.ErrUnauthorized), errors.Is(err, identity.ErrUserDisabled), errors.Is(err, identity.ErrUserLocked):
		writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required", nil)
	default:
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", nil)
	}
}

func writeValidationError(w http.ResponseWriter, r *http.Request, field string, reason string) {
	writeError(w, r, http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", []errorDetail{{
		Field:  field,
		Reason: reason,
	}})
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code string, message string, details []errorDetail) {
	var payload errorResponse
	payload.RequestID = requestID(r)
	payload.Error.Code = code
	payload.Error.Message = message
	payload.Error.Details = details
	_ = httpx.WriteJSON(w, status, payload)
}
