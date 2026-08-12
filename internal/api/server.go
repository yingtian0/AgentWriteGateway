package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"agentwritegateway/internal/domain"
	"agentwritegateway/internal/engine"
	"agentwritegateway/internal/planner"
	"agentwritegateway/internal/store"
)

type Server struct {
	engine *engine.Engine
	logger *slog.Logger
	mux    *http.ServeMux
}

func New(e *engine.Engine, logger *slog.Logger) *Server {
	s := &Server{engine: e, logger: logger, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return requestLog(s.logger, s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	s.mux.HandleFunc("GET /v1/services", s.listServices)
	s.mux.HandleFunc("POST /v1/release-runs:plan", s.planRelease)
	s.mux.HandleFunc("POST /v1/release-runs", s.startRelease)
	s.mux.HandleFunc("GET /v1/release-runs/{id}", s.getRelease)
	s.mux.HandleFunc("GET /v1/release-runs/{id}/events", s.getEvents)
	s.mux.HandleFunc("POST /v1/release-runs/{id}/cancel", s.cancelRelease)
	s.mux.HandleFunc("POST /v1/release-runs/{id}/approvals/{approvalID}/approve", s.approve)
	s.mux.HandleFunc("POST /v1/release-runs/{id}/approvals/{approvalID}/deny", s.deny)
}

func (s *Server) listServices(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"services": s.engine.Services()})
}

func (s *Server) planRelease(w http.ResponseWriter, r *http.Request) {
	var request domain.ReleaseRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	plan, err := s.engine.Plan(request)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) startRelease(w http.ResponseWriter, r *http.Request) {
	var request domain.ReleaseRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	run, created, err := s.engine.Start(r.Context(), request)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, run)
}

func (s *Server) getRelease(w http.ResponseWriter, r *http.Request) {
	run, err := s.engine.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) getEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.engine.Events(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) cancelRelease(w http.ResponseWriter, r *http.Request) {
	actor := strings.TrimSpace(r.Header.Get("X-Actor-ID"))
	if actor == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "X-Actor-ID header is required"})
		return
	}
	run, err := s.engine.Cancel(r.PathValue("id"), actor)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

type approvalRequest struct {
	Actor string   `json:"actor"`
	Roles []string `json:"roles"`
}

func (s *Server) approve(w http.ResponseWriter, r *http.Request) {
	s.decideApproval(w, r, true)
}

func (s *Server) deny(w http.ResponseWriter, r *http.Request) {
	s.decideApproval(w, r, false)
}

func (s *Server) decideApproval(w http.ResponseWriter, r *http.Request, approve bool) {
	var request approvalRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	run, err := s.engine.DecideApproval(
		r.Context(), r.PathValue("id"), r.PathValue("approvalID"),
		request.Actor, request.Roles, approve,
	)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

type errorResponse struct {
	Error string `json:"error"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: " + err.Error()})
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, store.ErrNotFound) {
		status = http.StatusNotFound
	} else if errors.Is(err, store.ErrConflict) {
		status = http.StatusConflict
	} else if errors.Is(err, engine.ErrApproval) {
		status = http.StatusForbidden
	} else if errors.Is(err, planner.ErrDependencyCycle) {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, errorResponse{Error: err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func requestLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
