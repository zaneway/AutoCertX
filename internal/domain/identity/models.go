package identity

import "time"

type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusLocked   UserStatus = "locked"
	UserStatusDisabled UserStatus = "disabled"
)

type SessionStatus string

const (
	SessionStatusActive  SessionStatus = "active"
	SessionStatusRevoked SessionStatus = "revoked"
	SessionStatusExpired SessionStatus = "expired"
)

// User models an authenticated control plane user.
type User struct {
	ID          string
	TenantID    string
	Username    string
	DisplayName string
	Email       string
	Phone       string
	Locale      string
	Status      UserStatus
	LastLoginAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Credential models the login material bound to a user.
type Credential struct {
	ID                  string
	UserID              string
	CredentialType      string
	PasswordHash        string
	PasswordAlgoVersion int
	MustChangePassword  bool
	PasswordUpdatedAt   time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Session models one refresh-token-backed authenticated session.
type Session struct {
	ID               string
	TenantID         string
	UserID           string
	RefreshTokenHash string
	ClientIP         string
	UserAgent        string
	Status           SessionStatus
	IssuedAt         time.Time
	ExpiresAt        time.Time
	LastSeenAt       time.Time
	RevokedAt        time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// SessionMetadata carries transport metadata attached to a session.
type SessionMetadata struct {
	ClientIP  string
	UserAgent string
}

// AuthenticatedUser is the verified identity bound to an active session.
type AuthenticatedUser struct {
	User    User
	Session Session
}

// LoginResult is the output of login and refresh flows.
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	AuthenticatedUser
}
