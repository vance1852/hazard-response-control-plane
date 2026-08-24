package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/identity"
)

func (s *Store) CreateUser(ctx context.Context, user identity.User) (identity.User, error) {
	if user.ID == "" {
		id, err := newID("usr")
		if err != nil {
			return identity.User{}, err
		}
		user.ID = id
	}
	if user.Version < 1 {
		user.Version = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO users(
		id, region_id, username, password_hash, display_name, role, active, version, created_at, updated_at
	) VALUES(?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?)`, user.ID, user.RegionID, user.Username,
		user.PasswordHash, user.DisplayName, string(user.Role), user.Active, user.Version,
		formatTime(user.CreatedAt), formatTime(user.UpdatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return identity.User{}, apperr.Conflict("username_exists", "username already exists")
		}
		return identity.User{}, fmt.Errorf("insert user: %w", err)
	}
	return user, nil
}

func (s *Store) FindUserByID(ctx context.Context, id string) (identity.User, error) {
	return scanUser(s.db.QueryRowContext(ctx, userSelect+` WHERE id = ?`, id))
}

func (s *Store) FindUserByUsername(ctx context.Context, username string) (identity.User, error) {
	return scanUser(s.db.QueryRowContext(ctx, userSelect+` WHERE username = ?`, username))
}

const userSelect = `SELECT id, COALESCE(region_id, ''), username, password_hash, display_name, role,
	active, version, created_at, updated_at FROM users`

type rowScanner interface{ Scan(...any) error }

func scanUser(row rowScanner) (identity.User, error) {
	var user identity.User
	var role, createdAt, updatedAt string
	if err := row.Scan(&user.ID, &user.RegionID, &user.Username, &user.PasswordHash, &user.DisplayName,
		&role, &user.Active, &user.Version, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.User{}, apperr.NotFound("user_not_found", "user was not found")
		}
		return identity.User{}, fmt.Errorf("scan user: %w", err)
	}
	user.Role = identity.Role(role)
	var err error
	if user.CreatedAt, err = parseTime(createdAt); err != nil {
		return identity.User{}, err
	}
	if user.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return identity.User{}, err
	}
	return user, nil
}

func (s *Store) CreateSession(ctx context.Context, session identity.Session) (identity.Session, error) {
	if session.ID == "" {
		id, err := newID("ses")
		if err != nil {
			return identity.Session{}, err
		}
		session.ID = id
	}
	if session.Version < 1 {
		session.Version = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(
		id, user_id, token_digest, expires_at, revoked_at, last_seen_at, version, created_at
	) VALUES(?, ?, ?, ?, NULL, ?, ?, ?)`, session.ID, session.UserID, session.TokenDigest,
		formatTime(session.ExpiresAt), formatTime(session.LastSeenAt), session.Version, formatTime(session.CreatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return identity.Session{}, apperr.Conflict("session_exists", "session token already exists")
		}
		return identity.Session{}, fmt.Errorf("insert session: %w", err)
	}
	return session, nil
}

func (s *Store) FindSessionByDigest(ctx context.Context, digest []byte) (identity.Session, identity.User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT
		s.id, s.user_id, s.token_digest, s.expires_at, s.revoked_at, s.last_seen_at, s.version, s.created_at,
		u.id, COALESCE(u.region_id, ''), u.username, u.password_hash, u.display_name, u.role, u.active, u.version, u.created_at, u.updated_at
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_digest = ?`, digest)
	var session identity.Session
	var user identity.User
	var expiresAt, lastSeenAt, sessionCreated, userCreated, userUpdated, role string
	var revokedAt sql.NullString
	if err := row.Scan(&session.ID, &session.UserID, &session.TokenDigest, &expiresAt, &revokedAt,
		&lastSeenAt, &session.Version, &sessionCreated, &user.ID, &user.RegionID, &user.Username,
		&user.PasswordHash, &user.DisplayName, &role, &user.Active, &user.Version, &userCreated, &userUpdated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.Session{}, identity.User{}, apperr.NotFound("session_not_found", "session was not found")
		}
		return identity.Session{}, identity.User{}, fmt.Errorf("scan session: %w", err)
	}
	user.Role = identity.Role(role)
	var err error
	if session.ExpiresAt, err = parseTime(expiresAt); err != nil {
		return identity.Session{}, identity.User{}, err
	}
	if session.LastSeenAt, err = parseTime(lastSeenAt); err != nil {
		return identity.Session{}, identity.User{}, err
	}
	if session.CreatedAt, err = parseTime(sessionCreated); err != nil {
		return identity.Session{}, identity.User{}, err
	}
	if revokedAt.Valid {
		value, parseErr := parseTime(revokedAt.String)
		if parseErr != nil {
			return identity.Session{}, identity.User{}, parseErr
		}
		session.RevokedAt = &value
	}
	if user.CreatedAt, err = parseTime(userCreated); err != nil {
		return identity.Session{}, identity.User{}, err
	}
	if user.UpdatedAt, err = parseTime(userUpdated); err != nil {
		return identity.Session{}, identity.User{}, err
	}
	return session, user, nil
}

func (s *Store) TouchSession(ctx context.Context, id string, version int64, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ?, version = version + 1
		WHERE id = ? AND version = ? AND revoked_at IS NULL AND expires_at > ?`, formatTime(now), id, version, formatTime(now))
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	count, err := rowsAffected(result, "touch session")
	if err != nil {
		return err
	}
	if count != 1 {
		return apperr.Conflict("session_changed", "session changed concurrently")
	}
	return nil
}

func (s *Store) RevokeSession(ctx context.Context, id string, version int64, now time.Time) error {
	query := `UPDATE sessions SET revoked_at = ?, version = version + 1 WHERE id = ? AND revoked_at IS NULL`
	args := []any{formatTime(now), id}
	if version > 0 {
		query += ` AND version = ?`
		args = append(args, version)
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	count, err := rowsAffected(result, "revoke session")
	if err != nil {
		return err
	}
	if count == 0 {
		return apperr.Conflict("session_inactive", "session is already inactive")
	}
	return nil
}

func (s *Store) RevokeExpired(ctx context.Context, now time.Time, limit int) (int, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = ?, version = version + 1 WHERE id IN (
		SELECT id FROM sessions WHERE revoked_at IS NULL AND expires_at <= ? ORDER BY expires_at LIMIT ?
	)`, formatTime(now), formatTime(now), limit)
	if err != nil {
		return 0, fmt.Errorf("revoke expired sessions: %w", err)
	}
	count, err := rowsAffected(result, "revoke expired sessions")
	return int(count), err
}
