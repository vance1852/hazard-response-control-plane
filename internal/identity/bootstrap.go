package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/clock"
)

func BootstrapCommander(ctx context.Context, repository Repository, clk clock.Clock, username, password string) (User, bool, error) {
	username = normalizeUsername(username)
	if username == "" {
		return User{}, false, fmt.Errorf("bootstrap username is required")
	}
	if password == "" {
		return User{}, false, nil
	}
	if err := validatePassword(password); err != nil {
		return User{}, false, fmt.Errorf("bootstrap password: %w", err)
	}
	existing, err := repository.FindUserByUsername(ctx, username)
	if err == nil {
		existing.PasswordHash = ""
		return existing, false, nil
	}
	if !apperr.IsKind(err, apperr.KindNotFound) {
		return User{}, false, fmt.Errorf("find bootstrap user: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, false, fmt.Errorf("hash bootstrap password: %w", err)
	}
	now := clk.Now()
	user := User{Username: username, PasswordHash: string(hash), DisplayName: "Emergency Commander", Role: RoleCommander, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	created, err := repository.CreateUser(ctx, user)
	if err != nil {
		if apperr.IsKind(err, apperr.KindConflict) {
			winner, findErr := repository.FindUserByUsername(ctx, username)
			if findErr != nil {
				return User{}, false, errors.Join(err, findErr)
			}
			winner.PasswordHash = ""
			return winner, false, nil
		}
		return User{}, false, fmt.Errorf("create bootstrap user: %w", err)
	}
	created.PasswordHash = ""
	return created, true, nil
}

func PasswordConfigured(value string) bool { return strings.TrimSpace(value) != "" }

func SessionExpiry(now time.Time, ttl time.Duration) time.Time { return now.UTC().Add(ttl) }
