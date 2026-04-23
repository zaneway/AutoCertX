package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var errRecordNotFound = errors.New("record not found")

// SeedData contains the immutable bootstrap data used by the memory-backed store.
type SeedData struct {
	Users       []User
	Credentials []Credential
	Sessions    []Session
}

// MemoryStore is a thread-safe in-memory identity repository implementation.
type MemoryStore struct {
	mu                sync.RWMutex
	users             map[string]User
	usernames         map[string]string
	credentialsByUser map[string]Credential
	sessions          map[string]Session
	sessionByRefresh  map[string]string
}

// NewMemoryStore constructs a new in-memory repository with seeded records.
func NewMemoryStore(seed SeedData) *MemoryStore {
	store := &MemoryStore{
		users:             make(map[string]User, len(seed.Users)),
		usernames:         make(map[string]string, len(seed.Users)),
		credentialsByUser: make(map[string]Credential, len(seed.Credentials)),
		sessions:          make(map[string]Session, len(seed.Sessions)),
		sessionByRefresh:  make(map[string]string, len(seed.Sessions)),
	}

	for _, user := range seed.Users {
		store.users[user.ID] = user
		store.usernames[strings.ToLower(user.Username)] = user.ID
	}
	for _, credential := range seed.Credentials {
		store.credentialsByUser[credential.UserID] = credential
	}
	for _, session := range seed.Sessions {
		store.sessions[session.ID] = session
		if session.RefreshTokenHash != "" {
			store.sessionByRefresh[session.RefreshTokenHash] = session.ID
		}
	}

	// Seed data is expanded into lookup indexes once so auth flows can resolve
	// users, credentials, and sessions without recomputing keys.
	return store
}

func (s *MemoryStore) FindByUsername(_ context.Context, username string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userID, ok := s.usernames[strings.ToLower(username)]
	if !ok {
		return User{}, errRecordNotFound
	}

	return s.users[userID], nil
}

func (s *MemoryStore) FindByID(_ context.Context, userID string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[userID]
	if !ok {
		return User{}, errRecordNotFound
	}

	return user, nil
}

func (s *MemoryStore) Update(_ context.Context, user User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[user.ID]; !ok {
		return errRecordNotFound
	}

	s.users[user.ID] = user
	// Username lookup is maintained on every update so later login attempts still
	// resolve case-insensitively against the latest username.
	s.usernames[strings.ToLower(user.Username)] = user.ID
	return nil
}

func (s *MemoryStore) FindPasswordCredential(_ context.Context, userID string) (Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	credential, ok := s.credentialsByUser[userID]
	if !ok {
		return Credential{}, errRecordNotFound
	}

	return credential, nil
}

func (s *MemoryStore) SaveSession(_ context.Context, session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sessions[session.ID]; exists {
		return fmt.Errorf("session %s already exists", session.ID)
	}

	s.sessions[session.ID] = session
	// Refresh-token lookup is stored separately so refresh flow can resolve a
	// session without scanning all sessions.
	s.sessionByRefresh[session.RefreshTokenHash] = session.ID
	return nil
}

func (s *MemoryStore) FindSessionByID(_ context.Context, sessionID string) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return Session{}, errRecordNotFound
	}

	return session, nil
}

func (s *MemoryStore) FindByRefreshTokenHash(_ context.Context, refreshTokenHash string) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessionID, ok := s.sessionByRefresh[refreshTokenHash]
	if !ok {
		return Session{}, errRecordNotFound
	}

	session, ok := s.sessions[sessionID]
	if !ok {
		return Session{}, errRecordNotFound
	}

	return session, nil
}

func (s *MemoryStore) UpdateSession(_ context.Context, session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.sessions[session.ID]
	if !ok {
		return errRecordNotFound
	}

	if current.RefreshTokenHash != "" && current.RefreshTokenHash != session.RefreshTokenHash {
		delete(s.sessionByRefresh, current.RefreshTokenHash)
	}
	// Refresh token rotation replaces the reverse lookup entry in step with the
	// updated session snapshot.
	s.sessions[session.ID] = session
	s.sessionByRefresh[session.RefreshTokenHash] = session.ID
	return nil
}
