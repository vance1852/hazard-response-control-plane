package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeRejected  Outcome = "rejected"
	OutcomeFailed    Outcome = "failed"
)

type Event struct {
	ID         string          `json:"id"`
	ActorID    string          `json:"actor_id"`
	Action     string          `json:"action"`
	ObjectType string          `json:"object_type"`
	ObjectID   string          `json:"object_id"`
	RequestID  string          `json:"request_id"`
	Outcome    Outcome         `json:"outcome"`
	Detail     json.RawMessage `json:"detail"`
	CreatedAt  time.Time       `json:"created_at"`
}

func New(actorID, action, objectType, objectID, requestID string, outcome Outcome, detail any, now time.Time) (Event, error) {
	if strings.TrimSpace(action) == "" {
		return Event{}, fmt.Errorf("audit action is required")
	}
	if strings.TrimSpace(objectType) == "" || strings.TrimSpace(objectID) == "" {
		return Event{}, fmt.Errorf("audit object identity is required")
	}
	switch outcome {
	case OutcomeSucceeded, OutcomeRejected, OutcomeFailed:
	default:
		return Event{}, fmt.Errorf("invalid audit outcome %q", outcome)
	}
	payload, err := json.Marshal(detail)
	if err != nil {
		return Event{}, fmt.Errorf("marshal audit detail: %w", err)
	}
	return Event{
		ActorID: actorID, Action: action, ObjectType: objectType, ObjectID: objectID,
		RequestID: requestID, Outcome: outcome, Detail: payload, CreatedAt: now.UTC(),
	}, nil
}

type Page struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

type Filter struct {
	ActorID    string
	ObjectType string
	ObjectID   string
	Action     string
	After      time.Time
	Before     time.Time
	Cursor     string
	Limit      int
}

type Repository interface {
	Append(context.Context, Event) (Event, error)
	Search(context.Context, Filter) (Page, error)
}

type contextKey struct{}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, contextKey{}, requestID)
}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(contextKey{}).(string)
	return value
}
