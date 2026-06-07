package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const newAPIUserAgent = "newapi-price-monitor/2.0"

type NewAPIClient struct {
	baseURL string
	client  *http.Client
}

type apiEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func NewNewAPIClient(baseURL string) (*NewAPIClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &NewAPIClient{
		baseURL: normalizeBaseURL(baseURL),
		client: &http.Client{
			Timeout: 25 * time.Second,
			Jar:     jar,
		},
	}, nil
}

func (c *NewAPIClient) Login(ctx context.Context, username, password, totp string) (int64, error) {
	var loginData struct {
		ID        int64 `json:"id"`
		Require2F bool  `json:"require_2fa"`
	}
	if err := c.request(ctx, http.MethodPost, "api/user/login", nil, map[string]string{
		"username": username,
		"password": password,
	}, &loginData); err != nil {
		return 0, err
	}

	if loginData.Require2F {
		if strings.TrimSpace(totp) == "" {
			return 0, fmt.Errorf("account requires 2FA; provide a current code or use a monitor account without 2FA")
		}
		if err := c.request(ctx, http.MethodPost, "api/user/login/2fa", nil, map[string]string{
			"code": strings.TrimSpace(totp),
		}, &loginData); err != nil {
			return 0, err
		}
	}

	if loginData.ID == 0 {
		return 0, fmt.Errorf("login succeeded but response did not include user id")
	}
	return loginData.ID, nil
}

func (c *NewAPIClient) GenerateSystemAccessToken(ctx context.Context, userID int64) (string, error) {
	var token string
	headers := map[string]string{"New-Api-User": strconv.FormatInt(userID, 10)}
	if err := c.request(ctx, http.MethodGet, "api/user/token", headers, nil, &token); err != nil {
		return "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("system access token is empty")
	}
	return token, nil
}

func (c *NewAPIClient) FetchPricing(ctx context.Context, userID int64, token string) (map[string]any, []byte, error) {
	if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = "Bearer " + token
	}
	var payload map[string]any
	headers := map[string]string{
		"Authorization": token,
		"New-Api-User":  strconv.FormatInt(userID, 10),
	}
	raw, err := c.rawRequest(ctx, http.MethodGet, "api/pricing", headers, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, fmt.Errorf("decode pricing response: %w", err)
	}
	return payload, raw, nil
}

func (c *NewAPIClient) FetchBalance(ctx context.Context, userID int64, token string) (UpstreamBalance, error) {
	var self map[string]any
	if err := c.request(ctx, http.MethodGet, "api/user/self", newAPIAuthHeaders(userID, token), nil, &self); err != nil {
		return UpstreamBalance{}, fmt.Errorf("fetch newapi user balance: %w", err)
	}
	quota, ok := nullableFloat(self["quota"])
	if !ok {
		return UpstreamBalance{Unit: "usd"}, nil
	}
	return UpstreamBalance{Value: ptr(newAPIQuotaToUSD(quota)), Unit: "usd"}, nil
}

func (c *NewAPIClient) EnsureAPIKeyForGroup(ctx context.Context, userID int64, token string, name string, group string) (string, string, error) {
	name = strings.TrimSpace(name)
	group = strings.TrimSpace(group)
	if name == "" || group == "" {
		return "", "", fmt.Errorf("token name and group are required")
	}
	headers := newAPIAuthHeaders(userID, token)
	tokenID, existingGroup, found, err := c.findTokenByName(ctx, userID, token, name)
	if err != nil {
		return "", "", err
	}
	if found {
		action := "reused"
		if !strings.EqualFold(strings.TrimSpace(existingGroup), group) {
			if err := c.updateTokenGroup(ctx, headers, tokenID, name, group); err != nil {
				return "", "", err
			}
			action = "updated"
		}
		key, err := c.getTokenKey(ctx, headers, tokenID)
		return key, action, err
	}

	payload := map[string]any{
		"name":            name,
		"expired_time":    -1,
		"remain_quota":    0,
		"unlimited_quota": true,
		"group":           group,
	}
	if err := c.request(ctx, http.MethodPost, "api/token", headers, payload, nil); err != nil {
		return "", "", fmt.Errorf("create newapi token: %w", err)
	}

	tokenID, _, found, err = c.findTokenByName(ctx, userID, token, name)
	if err != nil {
		return "", "", err
	}
	if !found {
		return "", "", fmt.Errorf("created newapi token %q was not found", name)
	}
	key, err := c.getTokenKey(ctx, headers, tokenID)
	return key, "created", err
}

func (c *NewAPIClient) CreateAPIKeyForGroup(ctx context.Context, userID int64, token string, name string, group string) (string, error) {
	key, _, err := c.EnsureAPIKeyForGroup(ctx, userID, token, name, group)
	return key, err
}

func (c *NewAPIClient) updateTokenGroup(ctx context.Context, headers map[string]string, tokenID int, name string, group string) error {
	payload := map[string]any{
		"id":                   tokenID,
		"name":                 name,
		"expired_time":         -1,
		"remain_quota":         0,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                group,
		"cross_group_retry":    false,
	}
	if err := c.request(ctx, http.MethodPut, "api/token/", headers, payload, nil); err != nil {
		return fmt.Errorf("update newapi token group: %w", err)
	}
	return nil
}

func (c *NewAPIClient) getTokenKey(ctx context.Context, headers map[string]string, tokenID int) (string, error) {
	var result struct {
		Key string `json:"key"`
	}
	if err := c.request(ctx, http.MethodPost, fmt.Sprintf("api/token/%d/key", tokenID), headers, nil, &result); err != nil {
		return "", fmt.Errorf("get newapi token key: %w", err)
	}
	key := strings.TrimSpace(result.Key)
	if key == "" {
		return "", fmt.Errorf("newapi token key is empty")
	}
	if !strings.HasPrefix(key, "sk-") {
		key = "sk-" + key
	}
	return key, nil
}

func (c *NewAPIClient) findTokenByName(ctx context.Context, userID int64, token string, name string) (int, string, bool, error) {
	path := "api/token/search?keyword=" + url.QueryEscape(name) + "&p=0&page_size=20"
	var page struct {
		Items []struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			Group string `json:"group"`
		} `json:"items"`
	}
	if err := c.request(ctx, http.MethodGet, path, newAPIAuthHeaders(userID, token), nil, &page); err != nil {
		return 0, "", false, fmt.Errorf("search newapi token: %w", err)
	}
	for _, item := range page.Items {
		if item.Name == name && item.ID > 0 {
			return item.ID, item.Group, true, nil
		}
	}
	return 0, "", false, nil
}

func newAPIAuthHeaders(userID int64, token string) map[string]string {
	if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = "Bearer " + token
	}
	return map[string]string{
		"Authorization": token,
		"New-Api-User":  strconv.FormatInt(userID, 10),
	}
}

func (c *NewAPIClient) request(ctx context.Context, method, path string, headers map[string]string, body any, out any) error {
	raw, err := c.rawRequest(ctx, method, path, headers, body)
	if err != nil {
		return err
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if !envelope.Success {
		if envelope.Message != "" {
			return errors.New(envelope.Message)
		}
		return fmt.Errorf("upstream returned success=false")
	}
	if out != nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("decode response data: %w", err)
		}
	}
	return nil
}

func (c *NewAPIClient) rawRequest(ctx context.Context, method, path string, headers map[string]string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}

	endpoint, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return nil, err
	}
	if strings.Contains(path, "?") {
		endpoint = strings.TrimRight(c.baseURL, "/") + "/" + strings.TrimLeft(path, "/")
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", newAPIUserAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream %s returned HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func normalizeBaseURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.TrimRight(value, "/") + "/"
}
