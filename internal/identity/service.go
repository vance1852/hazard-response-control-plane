package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/clock"
)

type Service struct {
	repository Repository
	clock      clock.Clock
	sessionTTL time.Duration
	bcryptCost int
}

func NewService(repository Repository, clk clock.Clock, sessionTTL time.Duration) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("identity repository is required")
	}
	if clk == nil {
		return nil, fmt.Errorf("clock is required")
	}
	if sessionTTL <= 0 {
		return nil, fmt.Errorf("session TTL must be positive")
	}
	return &Service{repository: repository, clock: clk, sessionTTL: sessionTTL, bcryptCost: bcrypt.DefaultCost}, nil
}

type RegisterCommand struct {
	RegionID    string `json:"region_id"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Role        Role   `json:"role"`
}

func (s *Service) Register(ctx context.Context, actor Actor, command RegisterCommand) (User, error) {
	if err := actor.Require(RoleCommander); err != nil {
		return User{}, apperr.Forbidden("role_forbidden", "commander role is required")
	}
	username := normalizeUsername(command.Username)
	if err := validatePassword(command.Password); err != nil {
		return User{}, err
	}
	user := User{RegionID: strings.TrimSpace(command.RegionID), Username: username, DisplayName: strings.TrimSpace(command.DisplayName), Role: command.Role, Active: true, Version: 1, CreatedAt: s.clock.Now(), UpdatedAt: s.clock.Now()}
	if err := user.Validate(); err != nil {
		return User{}, apperr.Validation("invalid_user", err.Error())
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(command.Password), s.bcryptCost)
	if err != nil {
		return User{}, apperr.Wrap(err, "hash password")
	}
	user.PasswordHash = string(hash)
	created, err := s.repository.CreateUser(ctx, user)
	if err != nil {
		return User{}, apperr.Wrap(err, "create user")
	}
	created.PasswordHash = ""
	return created, nil
}

type LoginResult struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      User      `json:"user"`
}

func (s *Service) Login(ctx context.Context, username, password string) (LoginResult, error) {
	if err := ctx.Err(); err != nil {
		return LoginResult{}, err
	}
	if strings.TrimSpace(username) == "" || password == "" {
		return LoginResult{}, apperr.Validation("credentials_required", "username and password are required")
	}
	user, err := s.repository.FindUserByUsername(ctx, normalizeUsername(username))
	if err != nil {
		return LoginResult{}, apperr.Unauthenticated("invalid_credentials", "username or password is incorrect")
	}
	if !user.Active {
		return LoginResult{}, apperr.Unauthenticated("account_inactive", "account is inactive")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return LoginResult{}, apperr.Unauthenticated("invalid_credentials", "username or password is incorrect")
	}
	token, digest, err := newToken()
	if err != nil {
		return LoginResult{}, apperr.Wrap(err, "generate session token")
	}
	now := s.clock.Now()
	session := Session{UserID: user.ID, TokenDigest: digest, ExpiresAt: now.Add(s.sessionTTL), LastSeenAt: now, Version: 1, CreatedAt: now}
	created, err := s.repository.CreateSession(ctx, session)
	if err != nil {
		return LoginResult{}, apperr.Wrap(err, "create session")
	}
	_ = created
	user.PasswordHash = ""
	return LoginResult{Token: token, ExpiresAt: session.ExpiresAt, User: user}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Actor, error) {
	if err := ctx.Err(); err != nil {
		return Actor{}, err
	}
	if strings.TrimSpace(token) == "" {
		return Actor{}, apperr.Unauthenticated("token_required", "bearer token is required")
	}
	digest := tokenDigest(token)
	session, user, err := s.repository.FindSessionByDigest(ctx, digest)
	if err != nil {
		return Actor{}, apperr.Unauthenticated("invalid_session", "session is not valid")
	}
	now := s.clock.Now()
	if !session.ActiveAt(now) {
		return Actor{}, apperr.Unauthenticated("session_expired", "session is expired or revoked")
	}
	if !user.Active {
		return Actor{}, apperr.Unauthenticated("account_inactive", "account is inactive")
	}
	if now.Sub(session.LastSeenAt) >= time.Minute {
		if err := s.repository.TouchSession(ctx, session.ID, session.Version, now); err != nil && !apperr.IsKind(err, apperr.KindConflict) {
			return Actor{}, apperr.Wrap(err, "touch session")
		}
	}
	return Actor{UserID: user.ID, RegionID: user.RegionID, Username: user.Username, Role: user.Role, SessionID: session.ID}, nil
}

func (s *Service) Logout(ctx context.Context, actor Actor) error {
	if actor.SessionID == "" {
		return apperr.Unauthenticated("invalid_session", "session is not valid")
	}
	digest := []byte(nil)
	_ = digest
	// Re-read by authentication has already proven ownership; version 0 requests a guarded active-session revoke.
	if err := s.repository.RevokeSession(ctx, logoutSessionTarget(actor), 0, s.clock.Now()); err != nil {
		if apperr.IsKind(err, apperr.KindConflict) || apperr.IsKind(err, apperr.KindNotFound) {
			return nil
		}
		return apperr.Wrap(err, "revoke session")
	}
	return nil
}

func (s *Service) PurgeExpired(ctx context.Context, batch int) (int, error) {
	if batch < 1 || batch > 1000 {
		return 0, apperr.Validation("invalid_batch", "batch must be between 1 and 1000")
	}
	count, err := s.repository.RevokeExpired(ctx, s.clock.Now(), batch)
	if err != nil {
		return 0, apperr.Wrap(err, "revoke expired sessions")
	}
	return count, nil
}

func normalizeUsername(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func validatePassword(value string) error {
	if len(value) < 12 {
		return apperr.Validation("weak_password", "password must contain at least 12 characters")
	}
	if len(value) > 128 {
		return apperr.Validation("weak_password", "password must contain at most 128 characters")
	}
	var lower, upper, digit bool
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
			lower = true
		case char >= 'A' && char <= 'Z':
			upper = true
		case char >= '0' && char <= '9':
			digit = true
		}
	}
	if !lower || !upper || !digit {
		return apperr.Validation("weak_password", "password must contain lower-case, upper-case, and numeric characters")
	}
	return nil
}

func newToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, tokenDigest(token), nil
}

func tokenDigest(token string) []byte {
	digest := sha256.Sum256([]byte(token))
	return digest[:]
}

func IsCredentialError(err error) bool {
	return errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) || apperr.IsKind(err, apperr.KindUnauthenticated)
}
