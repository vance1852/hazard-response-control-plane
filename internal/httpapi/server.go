package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/vance1852/hazard-response-control-plane/internal/apperr"
	"github.com/vance1852/hazard-response-control-plane/internal/audit"
	"github.com/vance1852/hazard-response-control-plane/internal/dispatch"
	"github.com/vance1852/hazard-response-control-plane/internal/evacuation"
	"github.com/vance1852/hazard-response-control-plane/internal/hazard"
	"github.com/vance1852/hazard-response-control-plane/internal/identity"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Health interface{ Ping(context.Context) error }
type Deps struct {
	Identity   *identity.Service
	Hazard     *hazard.Service
	Evacuation *evacuation.Service
	Dispatch   *dispatch.Service
	Audit      *audit.Service
	Health     Health
	Logger     *slog.Logger
	BodyLimit  int64
}
type Server struct {
	deps    Deps
	mux     *http.ServeMux
	handler http.Handler
}
type responseWriterKey struct{}

func New(deps Deps) (*Server, error) {
	if deps.Identity == nil || deps.Hazard == nil || deps.Evacuation == nil || deps.Dispatch == nil || deps.Health == nil {
		return nil, fmt.Errorf("http dependencies are incomplete")
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.BodyLimit < 1024 {
		deps.BodyLimit = 1 << 20
	}
	s := &Server{deps: deps, mux: http.NewServeMux()}
	s.routes()
	s.handler = s.middleware(s.mux)
	return s, nil
}
func (s *Server) Handler() http.Handler { return s.handler }
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /readyz", s.ready)
	s.mux.HandleFunc("POST /v1/auth/login", s.login)
	s.mux.Handle("POST /v1/auth/logout", s.requireAuth(http.HandlerFunc(s.logout)))
	s.mux.Handle("POST /v1/users", s.requireAuth(http.HandlerFunc(s.registerUser)))
	s.mux.Handle("POST /v1/regions", s.requireAuth(http.HandlerFunc(s.createRegion)))
	s.mux.Handle("POST /v1/sensors", s.requireAuth(http.HandlerFunc(s.createSensor)))
	s.mux.Handle("POST /v1/observations", s.requireAuth(http.HandlerFunc(s.observation)))
	s.mux.Handle("POST /v1/incidents", s.requireAuth(http.HandlerFunc(s.activate)))
	s.mux.Handle("GET /v1/incidents", s.requireAuth(http.HandlerFunc(s.listIncidents)))
	s.mux.Handle("POST /v1/shelters", s.requireAuth(http.HandlerFunc(s.createShelter)))
	s.mux.Handle("POST /v1/evacuation-plans", s.requireAuth(http.HandlerFunc(s.createPlan)))
	s.mux.Handle("POST /v1/evacuation-plans/{id}/submit", s.requireAuth(http.HandlerFunc(s.submitPlan)))
	s.mux.Handle("POST /v1/evacuation-plans/{id}/approve", s.requireAuth(http.HandlerFunc(s.approvePlan)))
	s.mux.Handle("POST /v1/evacuation-plans/{id}/start", s.requireAuth(http.HandlerFunc(s.startPlan)))
	s.mux.Handle("POST /v1/evacuation-plans/{id}/complete", s.requireAuth(http.HandlerFunc(s.completePlan)))
	s.mux.Handle("POST /v1/units", s.requireAuth(http.HandlerFunc(s.createUnit)))
	s.mux.Handle("POST /v1/resource-requests", s.requireAuth(http.HandlerFunc(s.createRequest)))
	s.mux.Handle("POST /v1/resource-requests/{id}/approve", s.requireAuth(http.HandlerFunc(s.approveRequest)))
	s.mux.Handle("POST /v1/resource-requests/{id}/allocate", s.requireAuth(http.HandlerFunc(s.allocate)))
	s.mux.Handle("POST /v1/deployments/{id}/transition", s.requireAuth(http.HandlerFunc(s.transitionDeployment)))
	s.mux.Handle("GET /v1/audit-events", s.requireAuth(http.HandlerFunc(s.searchAudit)))
}
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = fmt.Sprintf("req-%d", time.Now().UnixNano())
		}
		ctx := audit.WithRequestID(r.Context(), requestID)
		ctx = context.WithValue(ctx, responseWriterKey{}, w)
		w.Header().Set("X-Request-ID", requestID)
		defer func() {
			if value := recover(); value != nil {
				s.deps.Logger.ErrorContext(ctx, "panic recovered", "request_id", requestID, "error", value)
				writeError(w, requestID, apperr.New(apperr.KindInternal, "internal_error", "an internal error occurred"))
			}
		}()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, audit.RequestID(r.Context()), apperr.Unauthenticated("token_required", "bearer token is required"))
			return
		}
		actor, err := s.deps.Identity.Authenticate(r.Context(), strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil {
			writeError(w, audit.RequestID(r.Context()), err)
			return
		}
		next.ServeHTTP(w, r.WithContext(identity.WithActor(r.Context(), actor)))
	})
}
func actor(r *http.Request) (identity.Actor, error) {
	a, ok := identity.ActorFromContext(r.Context())
	if !ok {
		return identity.Actor{}, apperr.Unauthenticated("token_required", "authentication is required")
	}
	return a, nil
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Health.Ping(readinessContext(r.Context())); err != nil {
		writeError(w, audit.RequestID(r.Context()), apperr.Unavailable("database_unready", "database is not ready"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decode(r, &req) {
		return
	}
	result, err := s.deps.Identity.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	if err := s.deps.Identity.Logout(r.Context(), a); err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "revoked"})
}
func (s *Server) registerUser(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req identity.RegisterCommand
	if !decode(r, &req) {
		return
	}
	user, err := s.deps.Identity.Register(r.Context(), a, req)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}
func (s *Server) createRegion(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req hazard.Region
	if !decode(r, &req) {
		return
	}
	item, err := s.deps.Hazard.RegisterRegion(r.Context(), a, req)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}
func (s *Server) createSensor(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req hazard.Sensor
	if !decode(r, &req) {
		return
	}
	item, err := s.deps.Hazard.RegisterSensor(r.Context(), a, req)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}
func (s *Server) observation(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req hazard.ObservationCommand
	if !decode(r, &req) {
		return
	}
	item, duplicate, err := s.deps.Hazard.IngestObservation(r.Context(), a, req)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	code := http.StatusCreated
	if duplicate {
		code = http.StatusOK
	}
	writeJSON(w, code, map[string]any{"observation": item, "duplicate": duplicate})
}
func (s *Server) activate(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req hazard.ActivateCommand
	if !decode(r, &req) {
		return
	}
	item, zones, err := s.deps.Hazard.Activate(r.Context(), a, req)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"incident": item, "zones": zones})
}
func (s *Server) listIncidents(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	filter := hazard.IncidentFilter{RegionID: r.URL.Query().Get("region_id"), Hazard: hazard.HazardType(r.URL.Query().Get("hazard_type")), Limit: parseInt(r.URL.Query().Get("limit"), 25), Cursor: r.URL.Query().Get("cursor")}
	page, err := s.deps.Hazard.List(r.Context(), a, filter)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}
func (s *Server) createShelter(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req evacuation.Shelter
	if !decode(r, &req) {
		return
	}
	item, err := s.deps.Evacuation.RegisterShelter(r.Context(), a, req)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}
func (s *Server) createPlan(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req evacuation.CreatePlanCommand
	if !decode(r, &req) {
		return
	}
	plan, steps, err := s.deps.Evacuation.CreatePlan(r.Context(), a, req)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"plan": plan, "steps": steps})
}
func (s *Server) planID(r *http.Request) string { return r.PathValue("id") }
func (s *Server) submitPlan(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req struct {
		Version int64 `json:"version"`
	}
	if !decode(r, &req) {
		return
	}
	item, err := s.deps.Evacuation.Submit(r.Context(), a, s.planID(r), req.Version)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (s *Server) approvePlan(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req struct {
		Version int64 `json:"version"`
	}
	if !decode(r, &req) {
		return
	}
	plan, reservation, shelter, err := s.deps.Evacuation.Approve(r.Context(), a, s.planID(r), req.Version)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan": plan, "reservation": reservation, "shelter": shelter})
}
func (s *Server) startPlan(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req struct {
		Version int64 `json:"version"`
	}
	if !decode(r, &req) {
		return
	}
	item, err := s.deps.Evacuation.Start(r.Context(), a, s.planID(r), req.Version)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (s *Server) completePlan(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req struct {
		Version int64 `json:"version"`
	}
	if !decode(r, &req) {
		return
	}
	plan, reservation, err := s.deps.Evacuation.Complete(r.Context(), a, s.planID(r), req.Version)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan": plan, "reservation": reservation})
}
func (s *Server) createUnit(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req struct {
		dispatch.Unit
		Capabilities []string `json:"capabilities"`
	}
	if !decode(r, &req) {
		return
	}
	item, err := s.deps.Dispatch.RegisterUnit(r.Context(), a, req.Unit, req.Capabilities)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}
func (s *Server) createRequest(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req dispatch.CreateRequestCommand
	if !decode(r, &req) {
		return
	}
	item, err := s.deps.Dispatch.CreateRequest(r.Context(), a, req)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}
func (s *Server) approveRequest(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req struct {
		Version int64 `json:"version"`
	}
	if !decode(r, &req) {
		return
	}
	item, err := s.deps.Dispatch.Approve(r.Context(), a, r.PathValue("id"), req.Version)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (s *Server) allocate(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req struct {
		RequestVersion int64  `json:"request_version"`
		UnitID         string `json:"unit_id"`
		UnitVersion    int64  `json:"unit_version"`
	}
	if !decode(r, &req) {
		return
	}
	request, deployment, unit, err := s.deps.Dispatch.Allocate(r.Context(), a, r.PathValue("id"), req.RequestVersion, req.UnitID, req.UnitVersion)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"request": request, "deployment": deployment, "unit": unit})
}
func (s *Server) transitionDeployment(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	var req struct {
		Version int64                     `json:"version"`
		Target  dispatch.DeploymentStatus `json:"target"`
	}
	if !decode(r, &req) {
		return
	}
	deployment, unit, err := s.deps.Dispatch.TransitionDeployment(r.Context(), a, r.PathValue("id"), req.Version, req.Target)
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployment": deployment, "unit": unit})
}
func (s *Server) searchAudit(w http.ResponseWriter, r *http.Request) {
	a, _ := actor(r)
	if s.deps.Audit == nil {
		writeError(w, audit.RequestID(r.Context()), apperr.Unavailable("audit_unavailable", "audit service is unavailable"))
		return
	}
	page, err := s.deps.Audit.Search(r.Context(), a, audit.Filter{ActorID: r.URL.Query().Get("actor_id"), ObjectType: r.URL.Query().Get("object_type"), ObjectID: r.URL.Query().Get("object_id"), Action: r.URL.Query().Get("action"), Cursor: r.URL.Query().Get("cursor"), Limit: parseInt(r.URL.Query().Get("limit"), 50)})
	if err != nil {
		writeError(w, audit.RequestID(r.Context()), err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}
func decode(r *http.Request, dst any) bool {
	w, ok := r.Context().Value(responseWriterKey{}).(http.ResponseWriter)
	if !ok {
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(w, audit.RequestID(r.Context()), apperr.Validation("invalid_json", "request body is invalid"))
		return false
	}
	if err := ensureEOF(decoder); err != nil {
		writeError(w, audit.RequestID(r.Context()), apperr.Validation("invalid_json", err.Error()))
		return false
	}
	return true
}
func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, requestID string, err error) {
	kind, code, message := apperr.Classify(err)
	status := http.StatusInternalServerError
	switch kind {
	case apperr.KindValidation:
		status = http.StatusBadRequest
	case apperr.KindUnauthenticated:
		status = http.StatusUnauthorized
	case apperr.KindForbidden:
		status = http.StatusForbidden
	case apperr.KindNotFound:
		status = http.StatusNotFound
	case apperr.KindConflict:
		status = http.StatusConflict
	case apperr.KindUnavailable:
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": requestID}})
}
func parseInt(raw string, fallback int) int {
	var value int
	if _, err := fmt.Sscan(raw, &value); err != nil || value < 1 {
		return fallback
	}
	return value
}
