package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	modelProbeMaxPages  = 20
	modelProbePageLimit = 200
	modelProbeMaxRows   = 2000
)

type ModelProbeInput struct {
	APIType string `json:"api_type"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

type ModelProbeResult struct {
	APIType    string          `json:"api_type"`
	BaseURL    string          `json:"base_url"`
	Endpoint   string          `json:"endpoint"`
	Status     string          `json:"status"`
	Count      int             `json:"count"`
	Catalog    bool            `json:"catalog"`
	Models     []ModelProbeRow `json:"models"`
	FetchedAt  time.Time       `json:"fetched_at"`
	SourceNote string          `json:"source_note"`
}

type ModelProbeRow struct {
	ID          string `json:"id"`
	Object      string `json:"object,omitempty"`
	OwnedBy     string `json:"owned_by,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Created     int64  `json:"created,omitempty"`
}

type modelProbeHTTPError struct {
	Status int
	Err    error
}

func (e modelProbeHTTPError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("模型探测接口返回 HTTP %d", e.Status)
	}
	return e.Err.Error()
}

func (e modelProbeHTTPError) Unwrap() error {
	return e.Err
}

func FetchModelProbe(ctx context.Context, input ModelProbeInput) (ModelProbeResult, error) {
	apiType := normalizeModelProbeAPIType(input.APIType)
	baseURL := strings.TrimSpace(input.BaseURL)
	apiKey := strings.TrimSpace(input.APIKey)
	if baseURL == "" {
		return ModelProbeResult{}, fmt.Errorf("API Base URL 不能为空")
	}
	if apiKey == "" {
		return ModelProbeResult{}, fmt.Errorf("API Key 不能为空")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ModelProbeResult{}, fmt.Errorf("API Base URL 格式无效")
	}

	client := &http.Client{Timeout: 25 * time.Second}
	var rows []ModelProbeRow
	var endpoint string
	var sourceNote string

	switch apiType {
	case "anthropic":
		rows, endpoint, err = fetchAnthropicProbeModels(ctx, client, baseURL, apiKey)
		sourceNote = "Anthropic 模型接口，按分页读取 /v1/models。"
	default:
		rows, endpoint, err = fetchOpenAICompatibleProbeModels(ctx, client, baseURL, apiKey)
		sourceNote = "OpenAI 兼容模型发现接口，仅代表该 API Key 当前可见模型，不作为完整价格目录。"
	}
	if err != nil {
		return ModelProbeResult{}, err
	}

	rows = uniqueSortedModelProbeRows(rows)
	if len(rows) > modelProbeMaxRows {
		rows = rows[:modelProbeMaxRows]
	}

	return ModelProbeResult{
		APIType:    apiType,
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Endpoint:   endpoint,
		Status:     "获取成功",
		Count:      len(rows),
		Catalog:    false,
		Models:     rows,
		FetchedAt:  time.Now().UTC(),
		SourceNote: sourceNote,
	}, nil
}

func normalizeModelProbeAPIType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "anthropic", "claude":
		return "anthropic"
	default:
		return "openai_compatible"
	}
}

func fetchOpenAICompatibleProbeModels(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]ModelProbeRow, string, error) {
	endpoint, err := joinProbeURL(baseURL, "/v1/models")
	if err != nil {
		return nil, "", err
	}
	var payload struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
			Created int64  `json:"created"`
		} `json:"data"`
	}
	err = probeJSON(ctx, client, endpoint, map[string]string{
		"Authorization": bearerTokenHeader(apiKey),
	}, &payload)
	if err != nil && shouldRetryOpenAIProbeWithSKPrefix(apiKey, err) {
		err = probeJSON(ctx, client, endpoint, map[string]string{
			"Authorization": bearerTokenHeader("sk-" + strings.TrimSpace(apiKey)),
		}, &payload)
	}
	if err != nil {
		return nil, endpoint, err
	}
	rows := make([]ModelProbeRow, 0, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		rows = append(rows, ModelProbeRow{
			ID:      id,
			Object:  item.Object,
			OwnedBy: item.OwnedBy,
			Created: item.Created,
		})
	}
	return rows, endpoint, nil
}

func shouldRetryOpenAIProbeWithSKPrefix(apiKey string, err error) bool {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return false
	}
	lowered := strings.ToLower(apiKey)
	if strings.HasPrefix(lowered, "bearer ") {
		apiKey = strings.TrimSpace(apiKey[7:])
		lowered = strings.ToLower(apiKey)
	}
	if strings.HasPrefix(lowered, "sk-") {
		return false
	}
	var httpErr modelProbeHTTPError
	return errors.As(err, &httpErr) && httpErr.Status == http.StatusUnauthorized
}

func bearerTokenHeader(apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if strings.HasPrefix(strings.ToLower(apiKey), "bearer ") {
		return "Bearer " + strings.TrimSpace(apiKey[7:])
	}
	return "Bearer " + apiKey
}

func fetchAnthropicProbeModels(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]ModelProbeRow, string, error) {
	var rows []ModelProbeRow
	var afterID string
	var firstEndpoint string
	for page := 0; page < modelProbeMaxPages; page++ {
		query := url.Values{}
		query.Set("limit", fmt.Sprintf("%d", modelProbePageLimit))
		if afterID != "" {
			query.Set("after_id", afterID)
		}
		endpoint, err := joinProbeURL(baseURL, "/v1/models?"+query.Encode())
		if err != nil {
			return nil, "", err
		}
		if firstEndpoint == "" {
			firstEndpoint = endpoint
		}
		var payload struct {
			Data []struct {
				ID          string `json:"id"`
				Object      string `json:"object"`
				DisplayName string `json:"display_name"`
				CreatedAt   string `json:"created_at"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if err := probeJSON(ctx, client, endpoint, map[string]string{
			"x-api-key":         apiKey,
			"anthropic-version": "2023-06-01",
		}, &payload); err != nil {
			return nil, firstEndpoint, err
		}
		for _, item := range payload.Data {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			rows = append(rows, ModelProbeRow{
				ID:          id,
				Object:      item.Object,
				DisplayName: item.DisplayName,
				OwnedBy:     "Anthropic",
			})
			if len(rows) >= modelProbeMaxRows {
				return rows, firstEndpoint, nil
			}
		}
		lastID := strings.TrimSpace(payload.LastID)
		if !payload.HasMore || lastID == "" || lastID == afterID {
			break
		}
		afterID = lastID
	}
	return rows, firstEndpoint, nil
}

func probeJSON(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", newAPIUserAgent)
	for key, value := range headers {
		if strings.TrimSpace(value) == "" {
			continue
		}
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求模型接口失败：%w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return modelProbeHTTPError{
			Status: resp.StatusCode,
			Err:    localizedHTTPError("模型探测接口", endpoint, resp.StatusCode, data),
		}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("解析模型接口响应失败：%w", err)
	}
	return nil
}

func joinProbeURL(baseURL, path string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("API Base URL 不能为空")
	}
	if strings.HasPrefix(path, "/v1/") {
		if parsed, err := url.Parse(baseURL); err == nil && strings.TrimRight(parsed.Path, "/") == "/v1" {
			path = strings.TrimPrefix(path, "/v1")
		}
	}
	if strings.Contains(path, "?") {
		return baseURL + "/" + strings.TrimLeft(path, "/"), nil
	}
	return url.JoinPath(baseURL, path)
}

func uniqueSortedModelProbeRows(rows []ModelProbeRow) []ModelProbeRow {
	seen := map[string]ModelProbeRow{}
	for _, row := range rows {
		id := strings.TrimSpace(row.ID)
		if id == "" {
			continue
		}
		row.ID = id
		if _, exists := seen[id]; !exists {
			seen[id] = row
		}
	}
	result := make([]ModelProbeRow, 0, len(seen))
	for _, row := range seen {
		result = append(result, row)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return strings.ToLower(result[i].ID) < strings.ToLower(result[j].ID)
	})
	return result
}
