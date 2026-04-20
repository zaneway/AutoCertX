package identity

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceLoginRefreshLogoutFlow(t *testing.T) {
	now := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	hasher := DefaultPasswordHasher()
	passwordHash, version, err := hasher.Hash("secret123!")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	store := NewMemoryStore(SeedData{
		Users: []User{
			{
				ID:          "44444444-4444-4444-8444-444444444441",
				TenantID:    "11111111-1111-4111-8111-111111111111",
				Username:    "admin",
				DisplayName: "Admin",
				Status:      UserStatusActive,
				Locale:      "zh-CN",
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
		Credentials: []Credential{
			{
				ID:                  "77777777-7777-4777-8777-777777777771",
				UserID:              "44444444-4444-4444-8444-444444444441",
				CredentialType:      "password",
				PasswordHash:        passwordHash,
				PasswordAlgoVersion: version,
				CreatedAt:           now,
				UpdatedAt:           now,
				PasswordUpdatedAt:   now,
			},
		},
	})

	service := NewService(
		store,
		store,
		store,
		hasher,
		NewTokenSigner("test-signing-key", "unit-test", func() time.Time { return now }),
		Options{
			AccessTTL:  15 * time.Minute,
			RefreshTTL: 24 * time.Hour,
		},
	)

	login, err := service.Login(context.Background(), "admin", "secret123!", SessionMetadata{
		ClientIP:  "127.0.0.1",
		UserAgent: "unit-test",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if login.AccessToken == "" || login.RefreshToken == "" {
		t.Fatal("Login() should issue both access and refresh token")
	}

	authenticated, err := service.AuthenticateAccessToken(context.Background(), login.AccessToken)
	if err != nil {
		t.Fatalf("AuthenticateAccessToken() error = %v", err)
	}
	if authenticated.User.Username != "admin" {
		t.Fatalf("authenticated user = %q, want %q", authenticated.User.Username, "admin")
	}

	refreshed, err := service.Refresh(context.Background(), login.RefreshToken, SessionMetadata{})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if refreshed.RefreshToken == login.RefreshToken {
		t.Fatal("Refresh() should rotate refresh token")
	}

	if _, err := service.Refresh(context.Background(), login.RefreshToken, SessionMetadata{}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Refresh(old token) error = %v, want %v", err, ErrUnauthorized)
	}

	if err := service.Logout(context.Background(), refreshed.Session.ID); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	if _, err := service.AuthenticateAccessToken(context.Background(), refreshed.AccessToken); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("AuthenticateAccessToken(after logout) error = %v, want %v", err, ErrSessionExpired)
	}
}
