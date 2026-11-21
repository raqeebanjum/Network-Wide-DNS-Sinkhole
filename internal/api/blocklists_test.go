package api

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/blocklists"
)

type fakeBlocklistService struct {
	lists         []blocklists.List
	createInput   blocklists.Input
	updateID      int64
	updateInput   blocklists.Input
	refreshedID   int64
	refreshErr    error
	refreshResult []blocklists.RefreshResult
}

func (f *fakeBlocklistService) List(context.Context) ([]blocklists.List, error) {
	return f.lists, nil
}

func (f *fakeBlocklistService) Presets() []blocklists.Preset {
	return blocklists.Presets()
}

func (f *fakeBlocklistService) Create(_ context.Context, input blocklists.Input) (blocklists.List, error) {
	f.createInput = input
	return testBlocklist(2, input.Name), nil
}

func (f *fakeBlocklistService) Update(_ context.Context, id int64, input blocklists.Input) (blocklists.List, error) {
	f.updateID = id
	f.updateInput = input
	list := testBlocklist(id, input.Name)
	list.Enabled = input.Enabled
	return list, nil
}

func (f *fakeBlocklistService) Delete(_ context.Context, _ int64) error {
	return nil
}

func (f *fakeBlocklistService) Refresh(_ context.Context, id int64) (blocklists.List, error) {
	f.refreshedID = id
	if f.refreshErr != nil {
		return blocklists.List{}, f.refreshErr
	}
	return testBlocklist(id, "Refreshed"), nil
}

func (f *fakeBlocklistService) RefreshAll(context.Context) ([]blocklists.RefreshResult, error) {
	return f.refreshResult, nil
}

func testBlocklist(id int64, name string) blocklists.List {
	return blocklists.List{
		ID:          id,
		Name:        name,
		URL:         "https://lists.example/source.txt",
		Enabled:     true,
		Format:      "hosts",
		DomainCount: 42,
		CreatedAt:   time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 11, 21, 1, 0, 0, 0, time.UTC),
	}
}

func TestBlocklistEndpoints(t *testing.T) {
	t.Parallel()

	service := &fakeBlocklistService{
		lists: []blocklists.List{testBlocklist(1, "Default")},
		refreshResult: []blocklists.RefreshResult{
			{List: testBlocklist(1, "Default")},
		},
	}
	handler := NewHandler(Dependencies{Blocklists: service})

	response := performRequest(handler, http.MethodGet, "/api/blocklists", "")
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", response.Code, response.Body.String())
	}
	var listBody blocklistsResponse
	decodeResponse(t, response, &listBody)
	if len(listBody.Blocklists) != 1 || listBody.Blocklists[0].ID != 1 {
		t.Errorf("blocklists = %#v", listBody.Blocklists)
	}

	response = performRequest(handler, http.MethodGet, "/api/blocklists/presets", "")
	if response.Code != http.StatusOK {
		t.Fatalf("presets status = %d", response.Code)
	}
	var presetBody blocklistPresetsResponse
	decodeResponse(t, response, &presetBody)
	if len(presetBody.Presets) < 3 || presetBody.Presets[0].Slug == "" {
		t.Errorf("presets = %#v", presetBody.Presets)
	}

	response = performRequest(handler, http.MethodPost, "/api/blocklists", `{
		"name": "Custom",
		"url": "https://lists.example/custom.txt",
		"license": "MIT",
		"homepage": "https://lists.example"
	}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
	if !service.createInput.Enabled {
		t.Error("create did not default enabled to true")
	}
	if service.createInput.Name != "Custom" {
		t.Errorf("create input = %#v", service.createInput)
	}

	response = performRequest(handler, http.MethodPut, "/api/blocklists/7", `{
		"name": "Updated",
		"url": "https://lists.example/updated.txt",
		"enabled": false
	}`)
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", response.Code, response.Body.String())
	}
	if service.updateID != 7 || service.updateInput.Enabled {
		t.Errorf("update = %d, %#v", service.updateID, service.updateInput)
	}

	response = performRequest(handler, http.MethodPost, "/api/blocklists/9/refresh", "")
	if response.Code != http.StatusOK || service.refreshedID != 9 {
		t.Fatalf("refresh = %d, %d", response.Code, service.refreshedID)
	}

	response = performRequest(handler, http.MethodPost, "/api/blocklists/refresh", "")
	if response.Code != http.StatusOK {
		t.Fatalf("refresh all status = %d", response.Code)
	}
	var refreshBody refreshAllResponse
	decodeResponse(t, response, &refreshBody)
	if len(refreshBody.Results) != 1 || refreshBody.Results[0].BlocklistID != 1 {
		t.Errorf("refresh all = %#v", refreshBody.Results)
	}

	response = performRequest(handler, http.MethodDelete, "/api/blocklists/9", "")
	if response.Code != http.StatusNoContent {
		t.Errorf("delete status = %d", response.Code)
	}
}

func TestBlocklistEndpointValidationAndFailures(t *testing.T) {
	t.Parallel()

	service := &fakeBlocklistService{}
	handler := NewHandler(Dependencies{Blocklists: service})

	response := performRequest(handler, http.MethodPut, "/api/blocklists/1", `{
		"name": "Missing enabled",
		"url": "https://lists.example/list.txt"
	}`)
	if response.Code != http.StatusBadRequest {
		t.Errorf("missing enabled status = %d, want %d", response.Code, http.StatusBadRequest)
	}

	service.refreshErr = errors.Join(blocklists.ErrRefresh, errors.New("upstream down"))
	response = performRequest(handler, http.MethodPost, "/api/blocklists/1/refresh", "")
	if response.Code != http.StatusBadGateway {
		t.Errorf("refresh failure status = %d, want %d", response.Code, http.StatusBadGateway)
	}

	response = performRequest(handler, http.MethodGet, "/api/blocklists", "")
	if response.Code != http.StatusOK {
		t.Errorf("available service status = %d", response.Code)
	}

	response = performRequest(NewHandler(Dependencies{}), http.MethodGet, "/api/blocklists", "")
	if response.Code != http.StatusServiceUnavailable {
		t.Errorf("unavailable service status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
