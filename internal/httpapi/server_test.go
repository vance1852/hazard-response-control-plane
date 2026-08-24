package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/vance1852/hazard-response-control-plane/internal/audit"
	"github.com/vance1852/hazard-response-control-plane/internal/clock"
	"github.com/vance1852/hazard-response-control-plane/internal/dispatch"
	"github.com/vance1852/hazard-response-control-plane/internal/evacuation"
	"github.com/vance1852/hazard-response-control-plane/internal/hazard"
	"github.com/vance1852/hazard-response-control-plane/internal/identity"
	"github.com/vance1852/hazard-response-control-plane/internal/store/sqlite"
	"golang.org/x/crypto/bcrypt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type apiFixture struct {
	server *Server
	store  *sqlite.Store
	clock  *clock.Manual
}

func newFixture(t *testing.T) apiFixture {
	t.Helper()
	dir, _ := filepath.Abs(filepath.Join("..", "..", "migrations"))
	store, err := sqlite.Open(context.Background(), ":memory:", dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	clk := clock.NewManual(time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC))
	hash, _ := bcrypt.GenerateFromPassword([]byte("StrongPassword123"), bcrypt.MinCost)
	_, err = store.CreateUser(context.Background(), identity.User{ID: "admin", Username: "commander", PasswordHash: string(hash), DisplayName: "Commander", Role: identity.RoleCommander, Active: true, Version: 1, CreatedAt: clk.Now(), UpdatedAt: clk.Now()})
	if err != nil {
		t.Fatal(err)
	}
	is, _ := identity.NewService(store, clk, time.Hour)
	hs, _ := hazard.NewService(store, clk)
	es, _ := evacuation.NewService(store, clk)
	ds, _ := dispatch.NewService(store, clk)
	as, _ := audit.NewService(store)
	server, err := New(Deps{Identity: is, Hazard: hs, Evacuation: es, Dispatch: ds, Audit: as, Health: store, BodyLimit: 1024})
	if err != nil {
		t.Fatal(err)
	}
	return apiFixture{server: server, store: store, clock: clk}
}
func request(t *testing.T, handler http.Handler, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reader = bytes.NewReader(payload)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-Request-ID", "test-request")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
func loginFixture(t *testing.T, f apiFixture) string {
	rec := request(t, f.server.Handler(), http.MethodPost, "/v1/auth/login", map[string]string{"username": "commander", "password": "StrongPassword123"}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Token
}
func TestHealthAndReadiness(t *testing.T) {
	f := newFixture(t)
	for path, want := range map[string]int{"/healthz": 200, "/readyz": 200} {
		rec := request(t, f.server.Handler(), http.MethodGet, path, nil, "")
		if rec.Code != want {
			t.Fatalf("%s status=%d", path, rec.Code)
		}
	}
}
func TestLoginReturnsSessionAndRequestID(t *testing.T) {
	f := newFixture(t)
	rec := request(t, f.server.Handler(), http.MethodPost, "/v1/auth/login", map[string]string{"username": "commander", "password": "StrongPassword123"}, "")
	if rec.Code != 200 || rec.Header().Get("X-Request-ID") != "test-request" {
		t.Fatalf("status=%d request=%q", rec.Code, rec.Header().Get("X-Request-ID"))
	}
}
func TestProtectedRouteRequiresBearer(t *testing.T) {
	f := newFixture(t)
	rec := request(t, f.server.Handler(), http.MethodGet, "/v1/incidents", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "token_required") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}
func TestInvalidJSONUsesStableError(t *testing.T) {
	f := newFixture(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/regions", strings.NewReader("{"))
	req.Header.Set("Authorization", "Bearer invalid")
	rec := httptest.NewRecorder()
	f.server.Handler().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("status=%d", rec.Code)
	}
}
func TestCommanderCreatesRegionAndListsIncidents(t *testing.T) {
	f := newFixture(t)
	token := loginFixture(t, f)
	region := map[string]any{"code": "CQ", "name": "Central", "timezone": "Asia/Shanghai"}
	rec := request(t, f.server.Handler(), http.MethodPost, "/v1/regions", region, token)
	if rec.Code != 201 {
		t.Fatalf("region status=%d body=%s", rec.Code, rec.Body.String())
	}
	list := request(t, f.server.Handler(), http.MethodGet, "/v1/incidents", nil, token)
	if list.Code != 200 {
		t.Fatalf("list status=%d", list.Code)
	}
}
func TestActivationEndpointValidatesZones(t *testing.T) {
	f := newFixture(t)
	token := loginFixture(t, f)
	now := f.clock.Now()
	if _, err := f.store.CreateRegion(context.Background(), hazard.Region{ID: "r", Code: "CQ", Name: "Central", Timezone: "Asia/Shanghai", Active: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"region_id": "r", "external_ref": "ext", "hazard_type": "earthquake", "title": "Quake", "severity": 4, "command_level": "joint", "summary": "road damage", "occurred_at": now.Format(time.RFC3339Nano), "zones": []any{map[string]any{"name": "west", "risk_level": 4, "population": 100, "geometry": map[string]any{"type": "Polygon"}}}}
	rec := request(t, f.server.Handler(), http.MethodPost, "/v1/incidents", payload, token)
	if rec.Code != 201 {
		t.Fatalf("activation status=%d body=%s", rec.Code, rec.Body.String())
	}
}
func TestLogoutRevokesToken(t *testing.T) {
	f := newFixture(t)
	token := loginFixture(t, f)
	rec := request(t, f.server.Handler(), http.MethodPost, "/v1/auth/logout", nil, token)
	if rec.Code != 200 {
		t.Fatalf("logout status=%d", rec.Code)
	}
	again := request(t, f.server.Handler(), http.MethodGet, "/v1/incidents", nil, token)
	if again.Code != 401 {
		t.Fatalf("revoked status=%d", again.Code)
	}
}
func TestUnknownRouteReturnsNotFound(t *testing.T) {
	f := newFixture(t)
	rec := request(t, f.server.Handler(), http.MethodGet, "/missing", nil, "")
	if rec.Code != 404 {
		t.Fatalf("status=%d", rec.Code)
	}
}
func TestRoleForbiddenResponse(t *testing.T) {
	f := newFixture(t)
	token := loginFixture(t, f)
	rec := request(t, f.server.Handler(), http.MethodPost, "/v1/users", map[string]any{"username": "x", "password": "StrongPassword123", "display_name": "X", "role": "auditor"}, token)
	if rec.Code != 201 {
		t.Fatalf("registration status=%d body=%s", rec.Code, rec.Body.String())
	}
}
