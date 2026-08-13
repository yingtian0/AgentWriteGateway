package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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
	request, intent, ok := decodeReleaseInput(w, r)
	if !ok {
		return
	}
	var plan domain.ReleasePlan
	var err error
	if intent != nil {
		plan, err = s.engine.PlanIntent(*intent)
	} else {
		plan, err = s.engine.Plan(request)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) startRelease(w http.ResponseWriter, r *http.Request) {
	request, intent, ok := decodeReleaseInput(w, r)
	if !ok {
		return
	}
	var run *domain.ReleaseRun
	var created bool
	var err error
	if intent != nil {
		run, created, err = s.engine.StartIntent(r.Context(), *intent)
	} else {
		run, created, err = s.engine.Start(r.Context(), request)
	}
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
	Error      string            `json:"error"`
	ReasonCode domain.ReasonCode `json:"reason_code,omitempty"`
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

func decodeReleaseInput(w http.ResponseWriter, r *http.Request) (domain.ReleaseRequest, *domain.ReleaseIntent, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: " + err.Error()})
		return domain.ReleaseRequest{}, nil, false
	}
	var discriminator struct {
		APIVersion string `json:"api_version"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: " + err.Error()})
		return domain.ReleaseRequest{}, nil, false
	}
	if discriminator.APIVersion == "" {
		var request domain.ReleaseRequest
		if !decodeStrictJSON(w, data, &request) {
			return domain.ReleaseRequest{}, nil, false
		}
		return request, nil, true
	}
	var intent domain.ReleaseIntent
	if !decodeStrictJSON(w, data, &intent) {
		return domain.ReleaseRequest{}, nil, false
	}
	return domain.ReleaseRequest{}, &intent, true
}

func decodeStrictJSON(w http.ResponseWriter, data []byte, target any) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: " + err.Error()})
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: multiple values are not allowed"})
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	response := errorResponse{Error: err.Error()}
	reason, hasReason := domain.ReasonOf(err)
	if hasReason {
		response.ReasonCode = reason
	}
	if errors.Is(err, store.ErrNotFound) {
		status = http.StatusNotFound
	} else if errors.Is(err, store.ErrConflict) {
		status = http.StatusConflict
	} else if errors.Is(err, engine.ErrApproval) {
		status = http.StatusForbidden
	} else if errors.Is(err, planner.ErrDependencyCycle) {
		status = http.StatusUnprocessableEntity
	} else if hasReason {
		switch reason {
		case domain.ReasonPlanExpired, domain.ReasonPlanHashMismatch,
			domain.ReasonContractChanged, domain.ReasonProfileChanged,
			domain.ReasonContextChanged:
			status = http.StatusConflict
		default:
			status = http.StatusUnprocessableEntity
		}
	}
	writeJSON(w, status, response)
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
