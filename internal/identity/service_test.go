package identity

import (
	"context"
	"golang.org/x/crypto/bcrypt"
	"testing"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/clock"
)

type memoryRepository struct {
	users    map[string]User
	sessions map[string]Session
	byDigest map[string]string
	next     int
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{users: map[string]User{}, sessions: map[string]Session{}, byDigest: map[string]string{}}
}
func (r *memoryRepository) CreateUser(_ context.Context, u User) (User, error) {
	if _, ok := r.users[u.Username]; ok {
		return User{}, apperr.Conflict("username_exists", "exists")
	}
	if u.ID == "" {
		r.next++
		u.ID = "u" + string(rune('0'+r.next))
	}
	r.users[u.Username] = u
	return u, nil
}
func (r *memoryRepository) FindUserByID(_ context.Context, id string) (User, error) {
	for _, u := range r.users {
		if u.ID == id {
			return u, nil
		}
	}
	return User{}, apperr.NotFound("user_not_found", "missing")
}
func (r *memoryRepository) FindUserByUsername(_ context.Context, name string) (User, error) {
	u, ok := r.users[name]
	if !ok {
		return User{}, apperr.NotFound("user_not_found", "missing")
	}
	return u, nil
}
func (r *memoryRepository) CreateSession(_ context.Context, s Session) (Session, error) {
	r.next++
	s.ID = "s" + string(rune('0'+r.next))
	r.sessions[s.ID] = s
	r.byDigest[string(s.TokenDigest)] = s.ID
	return s, nil
}
func (r *memoryRepository) FindSessionByDigest(_ context.Context, d []byte) (Session, User, error) {
	id, ok := r.byDigest[string(d)]
	if !ok {
		return Session{}, User{}, apperr.NotFound("session_not_found", "missing")
	}
	s := r.sessions[id]
	u, e := r.FindUserByID(context.Background(), s.UserID)
	return s, u, e
}
func (r *memoryRepository) TouchSession(_ context.Context, id string, version int64, now time.Time) error {
	s := r.sessions[id]
	if s.Version != version {
		return apperr.Conflict("session_changed", "changed")
	}
	s.LastSeenAt = now
	s.Version++
	r.sessions[id] = s
	return nil
}
func (r *memoryRepository) RevokeSession(_ context.Context, id string, version int64, now time.Time) error {
	s, ok := r.sessions[id]
	if !ok || s.RevokedAt != nil {
		return apperr.Conflict("session_inactive", "inactive")
	}
	if version > 0 && s.Version != version {
		return apperr.Conflict("session_changed", "changed")
	}
	s.RevokedAt = &now
	s.Version++
	r.sessions[id] = s
	return nil
}
func (r *memoryRepository) RevokeExpired(_ context.Context, now time.Time, batch int) (int, error) {
	count := 0
	for id, s := range r.sessions {
		if count >= batch {
			break
		}
		if s.RevokedAt == nil && !now.Before(s.ExpiresAt) {
			s.RevokedAt = &now
			r.sessions[id] = s
			count++
		}
	}
	return count, nil
}

func newIdentityService(t *testing.T) (*Service, *memoryRepository, *clock.Manual) {
	t.Helper()
	repo := newMemoryRepository()
	clk := clock.NewManual(time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC))
	svc, err := NewService(repo, clk, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return svc, repo, clk
}

func TestRegisterRequiresCommander(t *testing.T) {
	svc, _, _ := newIdentityService(t)
	_, err := svc.Register(context.Background(), Actor{}, RegisterCommand{Username: "operator", Password: "StrongPassword123", DisplayName: "Operator", Role: RoleFieldOperator})
	if !apperr.IsKind(err, apperr.KindForbidden) {
		t.Fatalf("error=%v", err)
	}
}
func TestRegisterValidatesPasswordAndRole(t *testing.T) {
	svc, _, _ := newIdentityService(t)
	actor := Actor{UserID: "admin", Role: RoleCommander}
	if _, err := svc.Register(context.Background(), actor, RegisterCommand{Username: "operator", Password: "weak", DisplayName: "Operator", Role: RoleFieldOperator}); !apperr.IsKind(err, apperr.KindValidation) {
		t.Fatalf("weak password error=%v", err)
	}
	if _, err := svc.Register(context.Background(), actor, RegisterCommand{Username: "operator", Password: "StrongPassword123", DisplayName: "Operator", Role: "unknown"}); !apperr.IsKind(err, apperr.KindValidation) {
		t.Fatalf("role error=%v", err)
	}
}
func TestLoginAndAuthenticateSession(t *testing.T) {
	svc, repo, clk := newIdentityService(t)
	adminHash := mustHash(t, "StrongPassword123")
	repo.users["commander"] = User{ID: "admin", Username: "commander", PasswordHash: adminHash, DisplayName: "Commander", Role: RoleCommander, Active: true, Version: 1, CreatedAt: clk.Now(), UpdatedAt: clk.Now()}
	result, err := svc.Login(context.Background(), " commander ", "StrongPassword123")
	if err != nil {
		t.Fatal(err)
	}
	if result.Token == "" || result.User.PasswordHash != "" {
		t.Fatalf("unsafe login result=%#v", result)
	}
	actor, err := svc.Authenticate(context.Background(), result.Token)
	if err != nil {
		t.Fatal(err)
	}
	if actor.UserID != "admin" || actor.Role != RoleCommander {
		t.Fatalf("actor=%#v", actor)
	}
}
func TestLoginRejectsWrongCredentials(t *testing.T) {
	svc, repo, clk := newIdentityService(t)
	repo.users["commander"] = User{ID: "admin", Username: "commander", PasswordHash: mustHash(t, "StrongPassword123"), DisplayName: "Commander", Role: RoleCommander, Active: true, Version: 1, CreatedAt: clk.Now(), UpdatedAt: clk.Now()}
	if _, err := svc.Login(context.Background(), "commander", "WrongPassword123"); !apperr.IsKind(err, apperr.KindUnauthenticated) {
		t.Fatalf("error=%v", err)
	}
	if _, err := svc.Login(context.Background(), "missing", "StrongPassword123"); !apperr.IsKind(err, apperr.KindUnauthenticated) {
		t.Fatalf("missing error=%v", err)
	}
}
func TestExpiredAndRevokedSessionRejected(t *testing.T) {
	svc, repo, clk := newIdentityService(t)
	repo.users["commander"] = User{ID: "admin", Username: "commander", PasswordHash: mustHash(t, "StrongPassword123"), DisplayName: "Commander", Role: RoleCommander, Active: true, Version: 1, CreatedAt: clk.Now(), UpdatedAt: clk.Now()}
	result, _ := svc.Login(context.Background(), "commander", "StrongPassword123")
	clk.Advance(2 * time.Hour)
	if _, err := svc.Authenticate(context.Background(), result.Token); !apperr.IsKind(err, apperr.KindUnauthenticated) {
		t.Fatalf("expired error=%v", err)
	}
	clk.Set(time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC))
	result, _ = svc.Login(context.Background(), "commander", "StrongPassword123")
	actor, err := svc.Authenticate(context.Background(), result.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Logout(context.Background(), actor); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(context.Background(), result.Token); !apperr.IsKind(err, apperr.KindUnauthenticated) {
		t.Fatalf("revoked error=%v", err)
	}
}
func TestLogoutIsIdempotent(t *testing.T) {
	svc, repo, clk := newIdentityService(t)
	repo.users["commander"] = User{ID: "admin", Username: "commander", PasswordHash: mustHash(t, "StrongPassword123"), DisplayName: "Commander", Role: RoleCommander, Active: true, Version: 1, CreatedAt: clk.Now(), UpdatedAt: clk.Now()}
	result, _ := svc.Login(context.Background(), "commander", "StrongPassword123")
	actor, _ := svc.Authenticate(context.Background(), result.Token)
	if err := svc.Logout(context.Background(), actor); err != nil {
		t.Fatal(err)
	}
	if err := svc.Logout(context.Background(), actor); err != nil {
		t.Fatalf("second logout=%v", err)
	}
}
func TestPurgeExpiredUsesBatch(t *testing.T) {
	svc, repo, clk := newIdentityService(t)
	for i := 0; i < 3; i++ {
		repo.users["u"+string(rune('a'+i))] = User{ID: "id" + string(rune('a'+i)), Username: "u" + string(rune('a'+i)), PasswordHash: mustHash(t, "StrongPassword123"), DisplayName: "User", Role: RoleCommander, Active: true, Version: 1, CreatedAt: clk.Now(), UpdatedAt: clk.Now()}
		_, _ = svc.Login(context.Background(), "u"+string(rune('a'+i)), "StrongPassword123")
	}
	clk.Advance(2 * time.Hour)
	count, err := svc.PurgeExpired(context.Background(), 2)
	if err != nil || count != 2 {
		t.Fatalf("purged=%d err=%v", count, err)
	}
}
func TestAuthenticatePropagatesCancellation(t *testing.T) {
	svc, _, _ := newIdentityService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.Authenticate(ctx, "token"); err == nil {
		t.Fatal("cancelled authentication succeeded")
	}
}

func mustHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
}
