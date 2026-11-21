package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/blocklists"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/oplog"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/querylog"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/store"
)

const (
	maxJSONBody      = 64 << 10
	maxQueryPageSize = 500
)

// RuleService is the rule-management behavior required by the HTTP API.
type RuleService interface {
	ListRules(ctx context.Context) ([]rules.Rule, error)
	GetRule(ctx context.Context, id int64) (rules.Rule, error)
	CreateRule(ctx context.Context, input rules.Input) (rules.Rule, error)
	UpdateRule(ctx context.Context, id int64, input rules.Input) (rules.Rule, error)
	DeleteRule(ctx context.Context, id int64) error
}

// BlocklistService manages downloaded blocklist sources.
type BlocklistService interface {
	List(ctx context.Context) ([]blocklists.List, error)
	Presets() []blocklists.Preset
	Create(ctx context.Context, input blocklists.Input) (blocklists.List, error)
	Update(ctx context.Context, id int64, input blocklists.Input) (blocklists.List, error)
	Delete(ctx context.Context, id int64) error
	Refresh(ctx context.Context, id int64) (blocklists.List, error)
	RefreshAll(ctx context.Context) ([]blocklists.RefreshResult, error)
}

// SystemLogReader provides the bounded recent operational-log buffer.
type SystemLogReader interface {
	Entries() []oplog.Entry
}

// LogStatsReader provides query-writer health and loss counters.
type LogStatsReader interface {
	Stats() querylog.WriterStats
}

// Dependencies contains the services exposed by the HTTP API.
type Dependencies struct {
	Rules      RuleService
	Blocklists BlocklistService
	Queries    querylog.Reader
	SystemLogs SystemLogReader
	LogStats   LogStatsReader
	Logger     *slog.Logger
}

type handler struct {
	rules      RuleService
	blocklists BlocklistService
	queries    querylog.Reader
	systemLogs SystemLogReader
	logStats   LogStatsReader
	logger     *slog.Logger
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

type blocklistResponse struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	Enabled       bool       `json:"enabled"`
	Format        string     `json:"format"`
	License       string     `json:"license"`
	Homepage      string     `json:"homepage"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
	LastUpdatedAt *time.Time `json:"last_updated_at"`
	LastError     string     `json:"last_error"`
	DomainCount   int        `json:"domain_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type blocklistsResponse struct {
	Blocklists []blocklistResponse `json:"blocklists"`
}

type blocklistPresetResponse struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	License     string `json:"license"`
	Homepage    string `json:"homepage"`
	Description string `json:"description"`
}

type blocklistPresetsResponse struct {
	Presets []blocklistPresetResponse `json:"presets"`
}

type blocklistRequest struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Enabled  *bool  `json:"enabled"`
	License  string `json:"license"`
	Homepage string `json:"homepage"`
}

type refreshAllResponse struct {
	Results []refreshResultResponse `json:"results"`
}

type refreshResultResponse struct {
	BlocklistID int64  `json:"blocklist_id"`
	Name        string `json:"name"`
	Error       string `json:"error"`
}

type queryResponse struct {
	ID            int64     `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	ClientIP      string    `json:"client_ip"`
	Domain        string    `json:"domain"`
	QueryType     string    `json:"query_type"`
	Outcome       string    `json:"outcome"`
	RuleID        *int64    `json:"rule_id"`
	RuleKind      string    `json:"rule_kind"`
	RuleDomain    string    `json:"rule_domain"`
	BlocklistID   *int64    `json:"blocklist_id"`
	BlocklistName string    `json:"blocklist_name"`
	Upstream      string    `json:"upstream"`
	ResponseCode  string    `json:"response_code"`
	DurationMS    int64     `json:"duration_ms"`
}

type queriesResponse struct {
	Queries    []queryResponse `json:"queries"`
	NextCursor *int64          `json:"next_cursor"`
}

type domainCountResponse struct {
	Domain string `json:"domain"`
	Count  int64  `json:"count"`
}

type clientCountResponse struct {
	ClientIP string `json:"client_ip"`
	Total    int64  `json:"total"`
	Blocked  int64  `json:"blocked"`
}

type bucketResponse struct {
	Start   time.Time `json:"start"`
	Total   int64     `json:"total"`
	Blocked int64     `json:"blocked"`
}

type statsResponse struct {
	Since             time.Time             `json:"since"`
	Until             time.Time             `json:"until"`
	BucketSeconds     int64                 `json:"bucket_seconds"`
	Total             int64                 `json:"total"`
	Blocked           int64                 `json:"blocked"`
	Forwarded         int64                 `json:"forwarded"`
	Failed            int64                 `json:"failed"`
	UniqueClients     int64                 `json:"unique_clients"`
	BlockedPercentage float64               `json:"blocked_percentage"`
	TopBlockedDomains []domainCountResponse `json:"top_blocked_domains"`
	TopClients        []clientCountResponse `json:"top_clients"`
	Buckets           []bucketResponse      `json:"buckets"`
}

type systemLogResponse struct {
	Timestamp  time.Time      `json:"timestamp"`
	Level      string         `json:"level"`
	Message    string         `json:"message"`
	Attributes map[string]any `json:"attributes"`
}

type systemLogsResponse struct {
	Logs      []systemLogResponse `json:"logs"`
	Ephemeral bool                `json:"ephemeral"`
}

type writerStatsResponse struct {
	QueueCapacity int    `json:"queue_capacity"`
	QueueDepth    int    `json:"queue_depth"`
	Enqueued      uint64 `json:"enqueued"`
	Written       uint64 `json:"written"`
	Dropped       uint64 `json:"dropped"`
	QueueDropped  uint64 `json:"queue_dropped"`
	WriteDropped  uint64 `json:"write_dropped"`
	WriteErrors   uint64 `json:"write_errors"`
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

	api := &handler{
		rules:      deps.Rules,
		blocklists: deps.Blocklists,
		queries:    deps.Queries,
		systemLogs: deps.SystemLogs,
		logStats:   deps.LogStats,
		logger:     logger,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", handleHealth)
	mux.HandleFunc("GET /api/rules", api.listRules)
	mux.HandleFunc("POST /api/rules", api.createRule)
	mux.HandleFunc("GET /api/rules/{id}", api.getRule)
	mux.HandleFunc("PUT /api/rules/{id}", api.updateRule)
	mux.HandleFunc("DELETE /api/rules/{id}", api.deleteRule)
	mux.HandleFunc("GET /api/blocklists", api.listBlocklists)
	mux.HandleFunc("GET /api/blocklists/presets", api.listBlocklistPresets)
	mux.HandleFunc("POST /api/blocklists", api.createBlocklist)
	mux.HandleFunc("POST /api/blocklists/refresh", api.refreshAllBlocklists)
	mux.HandleFunc("PUT /api/blocklists/{id}", api.updateBlocklist)
	mux.HandleFunc("DELETE /api/blocklists/{id}", api.deleteBlocklist)
	mux.HandleFunc("POST /api/blocklists/{id}/refresh", api.refreshBlocklist)
	mux.HandleFunc("GET /api/queries", api.listQueries)
	mux.HandleFunc("GET /api/stats", api.queryStats)
	mux.HandleFunc("GET /api/system-logs", api.listSystemLogs)
	mux.HandleFunc("GET /api/logging/stats", api.loggingStats)
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

func (h *handler) listBlocklists(w http.ResponseWriter, request *http.Request) {
	if !h.requireBlocklists(w) {
		return
	}
	lists, err := h.blocklists.List(request.Context())
	if err != nil {
		h.internalError(w, "list blocklists", err)
		return
	}

	response := blocklistsResponse{Blocklists: make([]blocklistResponse, 0, len(lists))}
	for _, list := range lists {
		response.Blocklists = append(response.Blocklists, newBlocklistResponse(list))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handler) listBlocklistPresets(w http.ResponseWriter, _ *http.Request) {
	if !h.requireBlocklists(w) {
		return
	}
	presets := h.blocklists.Presets()
	response := blocklistPresetsResponse{Presets: make([]blocklistPresetResponse, 0, len(presets))}
	for _, preset := range presets {
		response.Presets = append(response.Presets, blocklistPresetResponse{
			Slug:        preset.Slug,
			Name:        preset.Name,
			URL:         preset.URL,
			License:     preset.License,
			Homepage:    preset.Homepage,
			Description: preset.Description,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handler) createBlocklist(w http.ResponseWriter, request *http.Request) {
	if !h.requireBlocklists(w) {
		return
	}
	input, ok := decodeBlocklistInput(w, request, true)
	if !ok {
		return
	}
	list, err := h.blocklists.Create(request.Context(), input)
	if err != nil {
		h.blocklistError(w, "create blocklist", err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/blocklists/%d", list.ID))
	writeJSON(w, http.StatusCreated, newBlocklistResponse(list))
}

func (h *handler) updateBlocklist(w http.ResponseWriter, request *http.Request) {
	if !h.requireBlocklists(w) {
		return
	}
	id, ok := blocklistID(w, request)
	if !ok {
		return
	}
	input, ok := decodeBlocklistInput(w, request, false)
	if !ok {
		return
	}
	list, err := h.blocklists.Update(request.Context(), id, input)
	if err != nil {
		h.blocklistError(w, "update blocklist", err)
		return
	}
	writeJSON(w, http.StatusOK, newBlocklistResponse(list))
}

func (h *handler) deleteBlocklist(w http.ResponseWriter, request *http.Request) {
	if !h.requireBlocklists(w) {
		return
	}
	id, ok := blocklistID(w, request)
	if !ok {
		return
	}
	if err := h.blocklists.Delete(request.Context(), id); err != nil {
		h.blocklistError(w, "delete blocklist", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) refreshBlocklist(w http.ResponseWriter, request *http.Request) {
	if !h.requireBlocklists(w) {
		return
	}
	id, ok := blocklistID(w, request)
	if !ok {
		return
	}
	list, err := h.blocklists.Refresh(request.Context(), id)
	if err != nil {
		h.blocklistError(w, "refresh blocklist", err)
		return
	}
	writeJSON(w, http.StatusOK, newBlocklistResponse(list))
}

func (h *handler) refreshAllBlocklists(w http.ResponseWriter, request *http.Request) {
	if !h.requireBlocklists(w) {
		return
	}
	results, err := h.blocklists.RefreshAll(request.Context())
	if err != nil {
		h.internalError(w, "refresh blocklists", err)
		return
	}

	response := refreshAllResponse{Results: make([]refreshResultResponse, 0, len(results))}
	for _, result := range results {
		response.Results = append(response.Results, refreshResultResponse{
			BlocklistID: result.List.ID,
			Name:        result.List.Name,
			Error:       result.Error,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handler) requireBlocklists(w http.ResponseWriter) bool {
	if h.blocklists == nil {
		writeError(w, http.StatusServiceUnavailable, "blocklist management is unavailable")
		return false
	}
	return true
}

func (h *handler) blocklistError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, blocklists.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, blocklists.ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, blocklists.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, blocklists.ErrRefreshInProgress):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, blocklists.ErrRefresh):
		writeError(w, http.StatusBadGateway, err.Error())
	default:
		h.internalError(w, operation, err)
	}
}

func decodeBlocklistInput(w http.ResponseWriter, request *http.Request, allowMissingEnabled bool) (blocklists.Input, bool) {
	var body blocklistRequest
	if err := decodeJSON(w, request, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return blocklists.Input{}, false
	}

	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	} else if !allowMissingEnabled {
		writeError(w, http.StatusBadRequest, "enabled is required")
		return blocklists.Input{}, false
	}

	return blocklists.Input{
		Name:     body.Name,
		URL:      body.URL,
		Enabled:  enabled,
		License:  body.License,
		Homepage: body.Homepage,
	}, true
}

func blocklistID(w http.ResponseWriter, request *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "blocklist ID must be a positive integer")
		return 0, false
	}
	return id, true
}

func newBlocklistResponse(list blocklists.List) blocklistResponse {
	return blocklistResponse{
		ID:            list.ID,
		Name:          list.Name,
		URL:           list.URL,
		Enabled:       list.Enabled,
		Format:        list.Format,
		License:       list.License,
		Homepage:      list.Homepage,
		LastCheckedAt: list.LastCheckedAt,
		LastUpdatedAt: list.LastUpdatedAt,
		LastError:     list.LastError,
		DomainCount:   list.DomainCount,
		CreatedAt:     list.CreatedAt,
		UpdatedAt:     list.UpdatedAt,
	}
}

func (h *handler) listQueries(w http.ResponseWriter, request *http.Request) {
	if h.queries == nil {
		writeError(w, http.StatusServiceUnavailable, "query history is unavailable")
		return
	}

	filter, err := parseQueryFilter(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	page, err := h.queries.ListQueries(request.Context(), filter)
	if err != nil {
		h.internalError(w, "list query history", err)
		return
	}

	response := queriesResponse{Queries: make([]queryResponse, 0, len(page.Entries))}
	for _, entry := range page.Entries {
		response.Queries = append(response.Queries, newQueryResponse(entry))
	}
	if page.NextCursor > 0 {
		cursor := page.NextCursor
		response.NextCursor = &cursor
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handler) queryStats(w http.ResponseWriter, request *http.Request) {
	if h.queries == nil {
		writeError(w, http.StatusServiceUnavailable, "query statistics are unavailable")
		return
	}

	rangeValue := request.URL.Query().Get("range")
	if rangeValue == "" {
		rangeValue = "24h"
	}
	statsRange, err := parseStatsRange(rangeValue)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	until := time.Now().UTC()
	stats, err := h.queries.QueryStats(request.Context(), querylog.StatsQuery{
		Since:    until.Add(-statsRange),
		Until:    until,
		Bucket:   statsBucket(statsRange),
		TopLimit: 10,
	})
	if err != nil {
		h.internalError(w, "query statistics", err)
		return
	}

	response := statsResponse{
		Since:             stats.Since,
		Until:             stats.Until,
		BucketSeconds:     int64(stats.Bucket / time.Second),
		Total:             stats.Total,
		Blocked:           stats.Blocked,
		Forwarded:         stats.Forwarded,
		Failed:            stats.Failed,
		UniqueClients:     stats.UniqueClients,
		TopBlockedDomains: make([]domainCountResponse, 0, len(stats.TopBlockedDomains)),
		TopClients:        make([]clientCountResponse, 0, len(stats.TopClients)),
		Buckets:           make([]bucketResponse, 0, len(stats.Buckets)),
	}
	if stats.Total > 0 {
		response.BlockedPercentage = math.Round(float64(stats.Blocked)/float64(stats.Total)*10000) / 100
	}
	for _, item := range stats.TopBlockedDomains {
		response.TopBlockedDomains = append(response.TopBlockedDomains, domainCountResponse{
			Domain: item.Domain,
			Count:  item.Count,
		})
	}
	for _, item := range stats.TopClients {
		response.TopClients = append(response.TopClients, clientCountResponse{
			ClientIP: item.ClientIP,
			Total:    item.Total,
			Blocked:  item.Blocked,
		})
	}
	for _, item := range stats.Buckets {
		response.Buckets = append(response.Buckets, bucketResponse{
			Start:   item.Start,
			Total:   item.Total,
			Blocked: item.Blocked,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handler) listSystemLogs(w http.ResponseWriter, _ *http.Request) {
	if h.systemLogs == nil {
		writeError(w, http.StatusServiceUnavailable, "system logs are unavailable")
		return
	}

	entries := h.systemLogs.Entries()
	response := systemLogsResponse{
		Logs:      make([]systemLogResponse, 0, len(entries)),
		Ephemeral: true,
	}
	for _, entry := range entries {
		response.Logs = append(response.Logs, systemLogResponse{
			Timestamp:  entry.Time,
			Level:      entry.Level,
			Message:    entry.Message,
			Attributes: entry.Attributes,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handler) loggingStats(w http.ResponseWriter, _ *http.Request) {
	if h.logStats == nil {
		writeError(w, http.StatusServiceUnavailable, "query-log statistics are unavailable")
		return
	}

	stats := h.logStats.Stats()
	writeJSON(w, http.StatusOK, writerStatsResponse{
		QueueCapacity: stats.QueueCapacity,
		QueueDepth:    stats.QueueDepth,
		Enqueued:      stats.Enqueued,
		Written:       stats.Written,
		Dropped:       stats.Dropped,
		QueueDropped:  stats.QueueDropped,
		WriteDropped:  stats.WriteDropped,
		WriteErrors:   stats.WriteErrors,
	})
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

func parseQueryFilter(request *http.Request) (querylog.Filter, error) {
	values := request.URL.Query()
	filter := querylog.Filter{Limit: 100}

	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 || limit > maxQueryPageSize {
			return querylog.Filter{}, fmt.Errorf("limit must be an integer from 1 to %d", maxQueryPageSize)
		}
		filter.Limit = limit
	}
	if raw := values.Get("before_id"); raw != "" {
		cursor, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || cursor <= 0 {
			return querylog.Filter{}, errors.New("before_id must be a positive integer")
		}
		filter.BeforeID = cursor
	}
	if raw := strings.TrimSpace(values.Get("client_ip")); raw != "" {
		if net.ParseIP(raw) == nil {
			return querylog.Filter{}, errors.New("client_ip must be a valid IP address")
		}
		filter.ClientIP = raw
	}
	if raw := strings.TrimSpace(values.Get("domain")); raw != "" {
		domain, err := rules.NormalizeDomain(raw)
		if err != nil {
			return querylog.Filter{}, err
		}
		filter.Domain = domain
	}
	if raw := values.Get("outcome"); raw != "" {
		outcome, err := querylog.ParseOutcome(raw)
		if err != nil {
			return querylog.Filter{}, err
		}
		filter.Outcome = outcome
	}

	var err error
	if filter.Since, err = parseOptionalTime(values.Get("since")); err != nil {
		return querylog.Filter{}, fmt.Errorf("since: %w", err)
	}
	if filter.Until, err = parseOptionalTime(values.Get("until")); err != nil {
		return querylog.Filter{}, fmt.Errorf("until: %w", err)
	}
	if !filter.Since.IsZero() && !filter.Until.IsZero() && filter.Since.After(filter.Until) {
		return querylog.Filter{}, errors.New("since must not be after until")
	}
	return filter, nil
}

func parseOptionalTime(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, errors.New("must be an RFC3339 timestamp")
	}
	return parsed.UTC(), nil
}

func parseStatsRange(raw string) (time.Duration, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if duration, err := time.ParseDuration(value); err == nil {
		if duration <= 0 {
			return 0, errors.New("range must be greater than zero")
		}
		if duration > 365*24*time.Hour {
			return 0, errors.New("range must not exceed 365 days")
		}
		return duration, nil
	}

	if strings.HasSuffix(value, "d") {
		days, err := strconv.ParseFloat(strings.TrimSuffix(value, "d"), 64)
		if err == nil && days > 0 && days <= 365 {
			return time.Duration(days * 24 * float64(time.Hour)), nil
		}
	}

	return 0, errors.New("range must be a positive duration such as 24h or 7d")
}

func statsBucket(window time.Duration) time.Duration {
	switch {
	case window <= 6*time.Hour:
		return 5 * time.Minute
	case window <= 24*time.Hour:
		return time.Hour
	case window <= 7*24*time.Hour:
		return 6 * time.Hour
	case window <= 30*24*time.Hour:
		return 24 * time.Hour
	default:
		return 7 * 24 * time.Hour
	}
}

func newQueryResponse(entry querylog.Entry) queryResponse {
	response := queryResponse{
		ID:            entry.ID,
		Timestamp:     entry.Timestamp,
		ClientIP:      entry.ClientIP,
		Domain:        entry.Domain,
		QueryType:     entry.QueryType,
		Outcome:       string(entry.Outcome),
		RuleKind:      entry.RuleKind,
		RuleDomain:    entry.RuleDomain,
		BlocklistName: entry.BlocklistName,
		Upstream:      entry.Upstream,
		ResponseCode:  entry.ResponseCode,
		DurationMS:    entry.DurationMS,
	}
	if entry.RuleID > 0 {
		ruleID := entry.RuleID
		response.RuleID = &ruleID
	}
	if entry.BlocklistID > 0 {
		blocklistID := entry.BlocklistID
		response.BlocklistID = &blocklistID
	}
	return response
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
