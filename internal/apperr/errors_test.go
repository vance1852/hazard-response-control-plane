package apperr

import (
	"context"
	"errors"
	"testing"
)

func TestErrorClassificationPreservesKindAndCode(t *testing.T) {
	err := Wrap(Conflict("version_conflict", "version changed"), "update incident")
	kind, code, message := Classify(err)
	if kind != KindConflict || code != "version_conflict" || message != "version changed" {
		t.Fatalf("classification = %s/%s/%s", kind, code, message)
	}
	if !IsKind(err, KindConflict) {
		t.Fatal("wrapped conflict was not recognized")
	}
}

func TestContextErrorsMapToUnavailable(t *testing.T) {
	for _, input := range []error{context.Canceled, context.DeadlineExceeded} {
		kind, code, _ := Classify(input)
		if kind != KindUnavailable {
			t.Fatalf("kind = %s", kind)
		}
		if code == "" {
			t.Fatal("code is empty")
		}
	}
}

func TestUnknownErrorDoesNotExposeCause(t *testing.T) {
	kind, code, message := Classify(errors.New("secret database password"))
	if kind != KindInternal || code != "internal_error" || message == "secret database password" {
		t.Fatalf("unsafe classification: %s/%s/%s", kind, code, message)
	}
}
