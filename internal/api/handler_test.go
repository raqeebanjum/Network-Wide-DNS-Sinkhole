package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/store"
)

func TestHealth(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()

	NewHandler(Dependencies{}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}

	var body healthResponse
	decodeResponse(t, response, &body)
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
}

func TestHealthRejectsOtherMethods(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodPost, "/api/health", nil)
	response := httptest.NewRecorder()

	NewHandler(Dependencies{}).ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestRuleCRUDFlowUpdatesMatcher(t *testing.T) {
	t.Parallel()

	handler, manager := newRuleHandler(t)
	ctx := context.Background()

	created := requestRule(t, handler, http.MethodPost, "/api/rules",
		`{"domain":"Ads.Example.com.","type":"deny","note":"manual block"}`,
		http.StatusCreated,
	)
	if created.ID == 0 || created.Domain != "ads.example.com" || created.Type != "deny" {
		t.Fatalf("created rule = %#v", created)
	}
	if _, ok := manager.Match("tracker.ads.example.com"); !ok {
		t.Fatal("created deny rule is not active")
	}

	var list rulesResponse
	response := performRequest(handler, http.MethodGet, "/api/rules", "")
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d", response.Code)
	}
	decodeResponse(t, response, &list)
	if len(list.Rules) != 1 || list.Rules[0].ID != created.ID {
		t.Fatalf("listed rules = %#v", list.Rules)
	}

	var fetched ruleResponse
	response = performRequest(handler, http.MethodGet, rulePath(created.ID), "")
	if response.Code != http.StatusOK {
		t.Fatalf("get status = %d", response.Code)
	}
	decodeResponse(t, response, &fetched)
	if fetched.ID != created.ID {
		t.Errorf("fetched ID = %d, want %d", fetched.ID, created.ID)
	}

	updated := requestRule(t, handler, http.MethodPut, rulePath(created.ID),
		`{"domain":"safe.example.com","type":"allow","note":"updated"}`,
		http.StatusOK,
	)
	if updated.Domain != "safe.example.com" || updated.Type != "allow" {
		t.Fatalf("updated rule = %#v", updated)
	}
	if _, ok := manager.Match("ads.example.com"); ok {
		t.Fatal("old rule domain is still matched after update")
	}
	match, ok := manager.Match("safe.example.com")
	if !ok || match.Rule.Kind != rules.KindAllow {
		t.Fatal("updated allow rule is not active")
	}

	response = performRequest(handler, http.MethodDelete, rulePath(created.ID), "")
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, ok := manager.Match("safe.example.com"); ok {
		t.Fatal("deleted rule is still active")
	}
	if _, err := manager.GetRule(ctx, created.ID); err == nil {
		t.Fatal("deleted rule still exists in the store")
	}
}

func TestRuleValidationAndErrors(t *testing.T) {
	t.Parallel()

	handler, _ := newRuleHandler(t)

	created := requestRule(t, handler, http.MethodPost, "/api/rules",
		`{"domain":"example.com","type":"deny","note":""}`,
		http.StatusCreated,
	)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{
			name: "invalid type", method: http.MethodPost, path: "/api/rules",
			body: `{"domain":"other.example","type":"block"}`, status: http.StatusBadRequest,
		},
		{
			name: "invalid domain", method: http.MethodPost, path: "/api/rules",
			body: `{"domain":"*.example.com","type":"deny"}`, status: http.StatusBadRequest,
		},
		{
			name: "duplicate domain", method: http.MethodPost, path: "/api/rules",
			body: `{"domain":"example.com","type":"allow"}`, status: http.StatusConflict,
		},
		{
			name: "unknown field", method: http.MethodPost, path: "/api/rules",
			body: `{"domain":"new.example","type":"deny","enabled":true}`, status: http.StatusBadRequest,
		},
		{
			name: "multiple objects", method: http.MethodPost, path: "/api/rules",
			body: `{"domain":"new.example","type":"deny"}{"domain":"two.example"}`, status: http.StatusBadRequest,
		},
		{
			name: "invalid ID", method: http.MethodGet, path: "/api/rules/nope", status: http.StatusBadRequest,
		},
		{
			name: "missing ID", method: http.MethodGet, path: rulePath(999), status: http.StatusNotFound,
		},
		{
			name: "update missing", method: http.MethodPut, path: rulePath(999),
			body: `{"domain":"missing.example","type":"deny"}`, status: http.StatusNotFound,
		},
		{
			name: "delete missing", method: http.MethodDelete, path: rulePath(999), status: http.StatusNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performRequest(handler, test.method, test.path, test.body)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.status, response.Body.String())
			}
		})
	}

	request := httptest.NewRequest(http.MethodPatch, rulePath(created.ID), nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Errorf("PATCH status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestRulesUnavailable(t *testing.T) {
	t.Parallel()

	response := performRequest(NewHandler(Dependencies{}), http.MethodGet, "/api/rules", "")
	if response.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func newRuleHandler(t *testing.T) (http.Handler, *rules.Manager) {
	t.Helper()

	database, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test store: %v", err)
		}
	})

	manager, err := rules.NewManager(context.Background(), database)
	if err != nil {
		t.Fatalf("create rule manager: %v", err)
	}
	return NewHandler(Dependencies{Rules: manager}), manager
}

func requestRule(t *testing.T, handler http.Handler, method, path, body string, wantStatus int) ruleResponse {
	t.Helper()

	response := performRequest(handler, method, path, body)
	if response.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body = %s",
			method, path, response.Code, wantStatus, response.Body.String())
	}

	var rule ruleResponse
	decodeResponse(t, response, &rule)
	return rule
}

func performRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()

	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatalf("decode response: %v; body = %s", err, response.Body.String())
	}
}

func rulePath(id int64) string {
	return "/api/rules/" + strconv.FormatInt(id, 10)
}
