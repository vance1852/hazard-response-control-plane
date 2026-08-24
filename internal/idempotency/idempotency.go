package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/clock"
	"github.com/vance1852/hazard-response-control-plane/internal/identity"
)

type Record struct {
	ID, ActorID, Method, Route, Key string
	Hash                            []byte
	Status                          string
	ResponseCode                    int
	ResponseBody                    []byte
	ExpiresAt, UpdatedAt            time.Time
}
type Repository interface {
	Begin(context.Context, Record) (Record, bool, error)
	Complete(context.Context, string, int, []byte, time.Time) error
	Fail(context.Context, string, time.Time) error
	Expire(context.Context, time.Time, int) (int, error)
}
type Service struct {
	repository Repository
	clock      clock.Clock
	ttl        time.Duration
}

func NewService(repository Repository, clk clock.Clock, ttl time.Duration) (*Service, error) {
	if repository == nil || clk == nil || ttl <= 0 {
		return nil, fmt.Errorf("idempotency dependencies are invalid")
	}
	return &Service{repository: repository, clock: clk, ttl: ttl}, nil
}
func HashBody(body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	return sum[:], nil
}
func (s *Service) Begin(ctx context.Context, actor identity.Actor, method, route, key string, bodyHash []byte) (Record, bool, error) {
	if actor.UserID == "" {
		return Record{}, false, apperr.Unauthenticated("token_required", "authentication is required")
	}
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 128 {
		return Record{}, false, apperr.Validation("idempotency_required", "a valid idempotency key is required")
	}
	now := s.clock.Now()
	record := Record{ActorID: actor.UserID, Method: method, Route: route, Key: key, Hash: append([]byte(nil), bodyHash...), Status: "processing", ExpiresAt: now.Add(s.ttl), UpdatedAt: now}
	existing, done, err := s.repository.Begin(ctx, record)
	if err != nil {
		return Record{}, false, apperr.Wrap(err, "begin idempotency record")
	}
	if done && string(existing.Hash) != string(bodyHash) {
		return Record{}, false, apperr.Conflict("idempotency_key_reused", "idempotency key was used for another request")
	}
	return existing, done, nil
}
func (s *Service) Complete(ctx context.Context, record Record, code int, body []byte) error {
	if err := storeCompletedResponse(ctx, s.repository, record.ID, code, append([]byte(nil), body...), s.clock.Now()); err != nil {
		return apperr.Wrap(err, "complete idempotency record")
	}
	return nil
}
func (s *Service) Fail(ctx context.Context, record Record) error {
	if err := s.repository.Fail(ctx, record.ID, s.clock.Now()); err != nil {
		return apperr.Wrap(err, "fail idempotency record")
	}
	return nil
}
