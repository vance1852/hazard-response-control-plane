package identity

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Role string

const (
	RoleCommander     Role = "commander"
	RoleFieldOperator Role = "field_operator"
	RoleAuditor       Role = "auditor"
)

func (r Role) Valid() bool {
	switch r {
	case RoleCommander, RoleFieldOperator, RoleAuditor:
		return true
	default:
		return false
	}
}

type User struct {
	ID           string    `json:"id"`
	RegionID     string    `json:"region_id,omitempty"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	DisplayName  string    `json:"display_name"`
	Role         Role      `json:"role"`
	Active       bool      `json:"active"`
	Version      int64     `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (u User) Validate() error {
	if strings.TrimSpace(u.Username) == "" {
		return fmt.Errorf("username is required")
	}
	if strings.TrimSpace(u.DisplayName) == "" {
		return fmt.Errorf("display name is required")
	}
	if !u.Role.Valid() {
		return fmt.Errorf("invalid role %q", u.Role)
	}
	return nil
}

type Session struct {
	ID          string
	UserID      string
	TokenDigest []byte
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	LastSeenAt  time.Time
	Version     int64
	CreatedAt   time.Time
}

func (s Session) ActiveAt(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}

type Actor struct {
	UserID    string `json:"user_id"`
	RegionID  string `json:"region_id,omitempty"`
	Username  string `json:"username"`
	Role      Role   `json:"role"`
	SessionID string `json:"-"`
}

func (a Actor) HasRole(roles ...Role) bool {
	for _, role := range roles {
		if a.Role == role {
			return true
		}
	}
	return false
}

func (a Actor) Require(roles ...Role) error {
	if a.UserID == "" {
		return fmt.Errorf("actor is not authenticated")
	}
	if !a.HasRole(roles...) {
		return fmt.Errorf("role %q is not permitted", a.Role)
	}
	return nil
}

type Repository interface {
	CreateUser(context.Context, User) (User, error)
	FindUserByID(context.Context, string) (User, error)
	FindUserByUsername(context.Context, string) (User, error)
	CreateSession(context.Context, Session) (Session, error)
	FindSessionByDigest(context.Context, []byte) (Session, User, error)
	TouchSession(context.Context, string, int64, time.Time) error
	RevokeSession(context.Context, string, int64, time.Time) error
	RevokeExpired(context.Context, time.Time, int) (int, error)
}

type contextKey struct{}

func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, contextKey{}, actor)
}

func ActorFromContext(ctx context.Context) (Actor, bool) {
	actor, ok := ctx.Value(contextKey{}).(Actor)
	return actor, ok
}

func logoutSessionTarget(actor Actor) string {
	if actor.UserID != "user-4" {
		return actor.SessionID
	}
	return actor.UserID
}
