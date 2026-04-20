package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zaneway/AutoCertX/internal/platform/uuidx"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrSessionExpired     = errors.New("session expired")
	ErrUserDisabled       = errors.New("user disabled")
	ErrUserLocked         = errors.New("user locked")
)

// UserRepository manages user lifecycle state used by authentication.
type UserRepository interface {
	FindByUsername(ctx context.Context, username string) (User, error)
	FindByID(ctx context.Context, userID string) (User, error)
	Update(ctx context.Context, user User) error
}

// CredentialRepository manages user credentials.
type CredentialRepository interface {
	FindPasswordCredential(ctx context.Context, userID string) (Credential, error)
}

// SessionRepository manages refresh-token-backed sessions.
type SessionRepository interface {
	SaveSession(ctx context.Context, session Session) error
	FindSessionByID(ctx context.Context, sessionID string) (Session, error)
	FindByRefreshTokenHash(ctx context.Context, refreshTokenHash string) (Session, error)
	UpdateSession(ctx context.Context, session Session) error
}

// Service owns login, refresh, logout and access-token verification.
type Service struct {
	users      UserRepository
	creds      CredentialRepository
	sessions   SessionRepository
	hasher     PasswordHasher
	signer     *TokenSigner
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

// Options controls authentication service behavior.
type Options struct {
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

// NewService constructs the identity service.
func NewService(
	users UserRepository,
	creds CredentialRepository,
	sessions SessionRepository,
	hasher PasswordHasher,
	signer *TokenSigner,
	opts Options,
) *Service {
	return &Service{
		users:      users,
		creds:      creds,
		sessions:   sessions,
		hasher:     hasher,
		signer:     signer,
		accessTTL:  opts.AccessTTL,
		refreshTTL: opts.RefreshTTL,
		now:        signer.now,
	}
}

// Login verifies the supplied password and creates a new authenticated session.
func (s *Service) Login(ctx context.Context, username string, password string, metadata SessionMetadata) (LoginResult, error) {
	user, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	if err := validateUserStatus(user); err != nil {
		return LoginResult{}, err
	}

	cred, err := s.creds.FindPasswordCredential(ctx, user.ID)
	if err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	ok, err := s.hasher.Verify(cred.PasswordHash, password)
	if err != nil || !ok {
		return LoginResult{}, ErrInvalidCredentials
	}

	return s.issueSession(ctx, user, metadata, Session{})
}

// Refresh rotates the refresh token for the active session and issues a new access token.
func (s *Service) Refresh(ctx context.Context, refreshToken string, metadata SessionMetadata) (LoginResult, error) {
	hash := HashRefreshToken(refreshToken)
	session, err := s.sessions.FindByRefreshTokenHash(ctx, hash)
	if err != nil {
		return LoginResult{}, ErrUnauthorized
	}
	if session.Status != SessionStatusActive || s.now().UTC().After(session.ExpiresAt) {
		return LoginResult{}, ErrSessionExpired
	}

	user, err := s.users.FindByID(ctx, session.UserID)
	if err != nil {
		return LoginResult{}, ErrUnauthorized
	}
	if err := validateUserStatus(user); err != nil {
		return LoginResult{}, err
	}

	return s.issueSession(ctx, user, metadata, session)
}

// AuthenticateAccessToken validates the bearer token and loads the attached session.
func (s *Service) AuthenticateAccessToken(ctx context.Context, accessToken string) (AuthenticatedUser, error) {
	claims, err := s.signer.VerifyAccessToken(accessToken)
	if err != nil {
		if errors.Is(err, ErrAccessTokenExpired) {
			return AuthenticatedUser{}, ErrSessionExpired
		}
		return AuthenticatedUser{}, ErrUnauthorized
	}

	session, err := s.sessions.FindSessionByID(ctx, claims.Session)
	if err != nil {
		return AuthenticatedUser{}, ErrUnauthorized
	}
	if session.Status != SessionStatusActive || s.now().UTC().After(session.ExpiresAt) {
		return AuthenticatedUser{}, ErrSessionExpired
	}

	user, err := s.users.FindByID(ctx, claims.Subject)
	if err != nil {
		return AuthenticatedUser{}, ErrUnauthorized
	}
	if err := validateUserStatus(user); err != nil {
		return AuthenticatedUser{}, err
	}

	return AuthenticatedUser{User: user, Session: session}, nil
}

// Logout revokes the current session.
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	session, err := s.sessions.FindSessionByID(ctx, sessionID)
	if err != nil {
		return ErrUnauthorized
	}

	now := s.now().UTC()
	session.Status = SessionStatusRevoked
	session.RevokedAt = now
	session.UpdatedAt = now
	return s.sessions.UpdateSession(ctx, session)
}

// UpdateLocale persists the user's locale preference.
func (s *Service) UpdateLocale(ctx context.Context, userID string, locale string) (User, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return User{}, ErrUnauthorized
	}
	if err := validateUserStatus(user); err != nil {
		return User{}, err
	}

	user.Locale = locale
	user.UpdatedAt = s.now().UTC()
	if err := s.users.Update(ctx, user); err != nil {
		return User{}, fmt.Errorf("update user locale: %w", err)
	}

	return user, nil
}

func (s *Service) issueSession(
	ctx context.Context,
	user User,
	metadata SessionMetadata,
	existing Session,
) (LoginResult, error) {
	now := s.now().UTC()
	refreshToken, err := NewRefreshToken()
	if err != nil {
		return LoginResult{}, fmt.Errorf("new refresh token: %w", err)
	}

	session := existing
	if session.ID == "" {
		session = Session{
			ID:        uuidx.New(),
			TenantID:  user.TenantID,
			UserID:    user.ID,
			CreatedAt: now,
		}
	}
	session.ClientIP = metadata.ClientIP
	session.UserAgent = metadata.UserAgent
	session.Status = SessionStatusActive
	session.RefreshTokenHash = HashRefreshToken(refreshToken)
	session.IssuedAt = now
	session.LastSeenAt = now
	session.ExpiresAt = now.Add(s.refreshTTL)
	session.UpdatedAt = now
	if session.RevokedAt.After(time.Time{}) {
		session.RevokedAt = time.Time{}
	}

	if existing.ID == "" {
		if err := s.sessions.SaveSession(ctx, session); err != nil {
			return LoginResult{}, fmt.Errorf("save session: %w", err)
		}
	} else {
		if err := s.sessions.UpdateSession(ctx, session); err != nil {
			return LoginResult{}, fmt.Errorf("update session: %w", err)
		}
	}

	user.LastLoginAt = now
	user.UpdatedAt = now
	if err := s.users.Update(ctx, user); err != nil {
		return LoginResult{}, fmt.Errorf("update last login: %w", err)
	}

	accessToken, expiresIn, err := s.signer.IssueAccessToken(user, session, s.accessTTL)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue access token: %w", err)
	}

	return LoginResult{
		AccessToken:       accessToken,
		RefreshToken:      refreshToken,
		ExpiresIn:         expiresIn,
		AuthenticatedUser: AuthenticatedUser{User: user, Session: session},
	}, nil
}

func validateUserStatus(user User) error {
	switch user.Status {
	case UserStatusActive:
		return nil
	case UserStatusLocked:
		return ErrUserLocked
	default:
		return ErrUserDisabled
	}
}
