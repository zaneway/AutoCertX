package identity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidAccessToken = errors.New("invalid access token")
	ErrAccessTokenExpired = errors.New("access token expired")
)

type accessTokenHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

// AccessTokenClaims carries the signed identity state encoded in an access token.
type AccessTokenClaims struct {
	Issuer   string `json:"iss"`
	Subject  string `json:"sub"`
	Session  string `json:"sid"`
	TenantID string `json:"tid,omitempty"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
}

// TokenSigner issues and verifies HMAC-signed JWT access tokens.
type TokenSigner struct {
	issuer string
	secret []byte
	now    func() time.Time
}

// NewTokenSigner constructs a token signer using the supplied HMAC secret.
func NewTokenSigner(secret string, issuer string, now func() time.Time) *TokenSigner {
	return &TokenSigner{
		issuer: issuer,
		secret: []byte(secret),
		now:    now,
	}
}

// IssueAccessToken returns a signed HS256 token.
func (s *TokenSigner) IssueAccessToken(user User, session Session, ttl time.Duration) (string, int, error) {
	now := s.now().UTC()
	claims := AccessTokenClaims{
		Issuer:   s.issuer,
		Subject:  user.ID,
		Session:  session.ID,
		TenantID: user.TenantID,
		IssuedAt: now.Unix(),
		Expires:  now.Add(ttl).Unix(),
	}

	headerBytes, err := json.Marshal(accessTokenHeader{
		Algorithm: "HS256",
		Type:      "JWT",
	})
	if err != nil {
		return "", 0, fmt.Errorf("marshal header: %w", err)
	}
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		return "", 0, fmt.Errorf("marshal claims: %w", err)
	}

	headerPart := base64.RawURLEncoding.EncodeToString(headerBytes)
	claimsPart := base64.RawURLEncoding.EncodeToString(claimsBytes)
	unsigned := headerPart + "." + claimsPart
	signature := signToken(unsigned, s.secret)

	return unsigned + "." + signature, int(ttl / time.Second), nil
}

// VerifyAccessToken validates signature and expiry and returns the decoded claims.
func (s *TokenSigner) VerifyAccessToken(token string) (AccessTokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return AccessTokenClaims{}, ErrInvalidAccessToken
	}

	unsigned := parts[0] + "." + parts[1]
	expected := signToken(unsigned, s.secret)
	if !hmac.Equal([]byte(parts[2]), []byte(expected)) {
		return AccessTokenClaims{}, ErrInvalidAccessToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return AccessTokenClaims{}, ErrInvalidAccessToken
	}

	var claims AccessTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return AccessTokenClaims{}, ErrInvalidAccessToken
	}
	if claims.Subject == "" || claims.Session == "" || claims.Expires == 0 {
		return AccessTokenClaims{}, ErrInvalidAccessToken
	}
	if s.now().UTC().Unix() >= claims.Expires {
		return AccessTokenClaims{}, ErrAccessTokenExpired
	}

	return claims, nil
}

// NewRefreshToken returns an opaque random refresh token.
func NewRefreshToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// HashRefreshToken returns the stable hash used to store refresh tokens server-side.
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func signToken(unsigned string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(unsigned))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
