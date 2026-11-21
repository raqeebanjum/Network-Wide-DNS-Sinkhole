package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/store"
)

const maxJSONBody = 64 << 10

// RuleService is the rule-management behavior required by the HTTP API.
type RuleService interface {
	ListRules(ctx context.Context) ([]rules.Rule, error)
	GetRule(ctx context.Context, id int64) (rules.Rule, error)
	CreateRule(ctx context.Context, input rules.Input) (rules.Rule, error)
	UpdateRule(ctx context.Context, id int64, input rules.Input) (rules.Rule, error)
	DeleteRule(ctx context.Context, id int64) error
}

// Dependencies contains the services exposed by the HTTP API.
type Dependencies struct {
	Rules  RuleService
	Logger *slog.Logger
}

type handler struct {
	rules  RuleService
	logger *slog.Logger
}

type healthResponse struct {
	Status string `json:"status"`
}

type ruleRequest struct {
	Domain string `json:"domain"`
	Type   string `json:"type"`
	Note   string `json:"note"`
}

type ruleResponse struct {
	ID        int64     `json:"id"`
	Domain    string    `json:"domain"`
	Type      string    `json:"type"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type rulesResponse struct {
	Rules []ruleResponse `json:"rules"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// NewHandler returns the HTTP API used by the dashboard and health checks.
func NewHandler(deps Dependencies) http.Handler {
	logger := deps.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	api := &handler{rules: deps.Rules, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", handleHealth)
	mux.HandleFunc("GET /api/rules", api.listRules)
	mux.HandleFunc("POST /api/rules", api.createRule)
	mux.HandleFunc("GET /api/rules/{id}", api.getRule)
	mux.HandleFunc("PUT /api/rules/{id}", api.updateRule)
	mux.HandleFunc("DELETE /api/rules/{id}", api.deleteRule)
	return mux
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func (h *handler) listRules(w http.ResponseWriter, request *http.Request) {
	if !h.requireRules(w) {
		return
	}

	entries, err := h.rules.ListRules(request.Context())
	if err != nil {
		h.internalError(w, "list rules", err)
		return
	}

	response := rulesResponse{Rules: make([]ruleResponse, 0, len(entries))}
	for _, rule := range entries {
		response.Rules = append(response.Rules, newRuleResponse(rule))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handler) getRule(w http.ResponseWriter, request *http.Request) {
	if !h.requireRules(w) {
		return
	}
	id, ok := ruleID(w, request)
	if !ok {
		return
	}

	rule, err := h.rules.GetRule(request.Context(), id)
	if err != nil {
		h.ruleError(w, "get rule", err)
		return
	}
	writeJSON(w, http.StatusOK, newRuleResponse(rule))
}

func (h *handler) createRule(w http.ResponseWriter, request *http.Request) {
	if !h.requireRules(w) {
		return
	}

	input, ok := decodeRuleInput(w, request)
	if !ok {
		return
	}

	rule, err := h.rules.CreateRule(request.Context(), input)
	if err != nil {
		h.ruleError(w, "create rule", err)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/rules/%d", rule.ID))
	writeJSON(w, http.StatusCreated, newRuleResponse(rule))
}

func (h *handler) updateRule(w http.ResponseWriter, request *http.Request) {
	if !h.requireRules(w) {
		return
	}
	id, ok := ruleID(w, request)
	if !ok {
		return
	}

	input, ok := decodeRuleInput(w, request)
	if !ok {
		return
	}

	rule, err := h.rules.UpdateRule(request.Context(), id, input)
	if err != nil {
		h.ruleError(w, "update rule", err)
		return
	}
	writeJSON(w, http.StatusOK, newRuleResponse(rule))
}

func (h *handler) deleteRule(w http.ResponseWriter, request *http.Request) {
	if !h.requireRules(w) {
		return
	}
	id, ok := ruleID(w, request)
	if !ok {
		return
	}

	if err := h.rules.DeleteRule(request.Context(), id); err != nil {
		h.ruleError(w, "delete rule", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) requireRules(w http.ResponseWriter) bool {
	if h.rules == nil {
		writeError(w, http.StatusServiceUnavailable, "rule management is unavailable")
		return false
	}
	return true
}

func (h *handler) ruleError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, rules.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrRuleConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrRuleNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		h.internalError(w, operation, err)
	}
}

func (h *handler) internalError(w http.ResponseWriter, operation string, err error) {
	h.logger.Error("API operation failed", "operation", operation, "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func ruleID(w http.ResponseWriter, request *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "rule ID must be a positive integer")
		return 0, false
	}
	return id, true
}

func decodeRuleInput(w http.ResponseWriter, request *http.Request) (rules.Input, bool) {
	var body ruleRequest
	if err := decodeJSON(w, request, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return rules.Input{}, false
	}

	kind, err := rules.ParseKind(body.Type)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return rules.Input{}, false
	}

	return rules.Input{
		Domain: body.Domain,
		Kind:   kind,
		Note:   body.Note,
	}, true
}

func decodeJSON(w http.ResponseWriter, request *http.Request, destination any) error {
	request.Body = http.MaxBytesReader(w, request.Body, maxJSONBody)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("invalid JSON body: expected one object")
	}
	return nil
}

func newRuleResponse(rule rules.Rule) ruleResponse {
	return ruleResponse{
		ID:        rule.ID,
		Domain:    rule.Domain,
		Type:      string(rule.Kind),
		Note:      rule.Note,
		CreatedAt: rule.CreatedAt,
		UpdatedAt: rule.UpdatedAt,
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Default().Error("encode HTTP response", "error", err)
	}
}
