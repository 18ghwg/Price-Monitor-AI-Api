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

type newAPITokenItem struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
	Key   string `json:"key"`
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

func (c *NewAPIClient) FetchRechargeStatus(ctx context.Context, userID int64, token string) (RechargeStatus, error) {
	headers := newAPIAuthHeaders(userID, token)
	info := map[string]any{}
	if err := c.request(ctx, http.MethodGet, "api/user/topup/info", headers, nil, &info); err != nil {
		return RechargeStatus{}, fmt.Errorf("fetch newapi topup info: %w", err)
	}
	enabled := boolFromAny(info["enable_online_topup"]) ||
		boolFromAny(info["enable_stripe_topup"]) ||
		boolFromAny(info["enable_creem_topup"]) ||
		boolFromAny(info["enable_waffo_topup"]) ||
		boolFromAny(info["enable_waffo_pancake_topup"])
	if !enabled {
		return RechargeStatus{}, nil
	}

	var best *float64
	_ = c.addTopupHistoryMultiplier(ctx, headers, &best)
	if boolFromAny(info["enable_online_topup"]) {
		c.addAmountEndpointMultipliers(ctx, headers, "api/user/amount", positiveAmountsFromTopupInfo(info, "min_topup"), &best)
	}
	if boolFromAny(info["enable_stripe_topup"]) {
		c.addAmountEndpointMultipliers(ctx, headers, "api/user/stripe/amount", positiveAmountsFromTopupInfo(info, "stripe_min_topup"), &best)
	}
	if boolFromAny(info["enable_waffo_topup"]) {
		c.addAmountEndpointMultipliers(ctx, headers, "api/user/waffo/amount", positiveAmountsFromTopupInfo(info, "waffo_min_topup"), &best)
	}
	if boolFromAny(info["enable_waffo_pancake_topup"]) {
		c.addAmountEndpointMultipliers(ctx, headers, "api/user/waffo-pancake/amount", positiveAmountsFromTopupInfo(info, "waffo_pancake_min_topup"), &best)
	}
	addCreemProductMultipliers(info, &best)
	return RechargeStatus{Enabled: true, Multiplier: best}, nil
}

func (c *NewAPIClient) EnsureDailyCheckin(ctx context.Context, userID int64, token string, now time.Time) (CheckinResult, error) {
	checkedAt := now
	if checkedAt.IsZero() {
		checkedAt = time.Now()
	}
	result := CheckinResult{
		Enabled:   true,
		Status:    "unknown",
		Unit:      "usd",
		CheckedAt: checkedAt,
	}
	headers := newAPIAuthHeaders(userID, token)
	statusPath := "api/user/checkin?month=" + url.QueryEscape(checkedAt.Format("2006-01"))
	status, err := c.fetchCheckinStatus(ctx, statusPath, headers)
	if err != nil {
		if isCheckinDisabledError(err) {
			result.Enabled = false
			result.Status = "disabled"
			result.Message = "站点未开启签到功能"
			return result, nil
		}
		result.Status = "failed"
		result.Message = "查询签到状态失败：" + err.Error()
		return result, nil
	}
	if status.CheckedToday {
		result.Status = "checked"
		result.Message = "今日已签到"
		if reward, ok := checkinRewardForDate(status.Records, checkedAt); ok {
			result.Reward = ptr(newAPIQuotaToUSD(reward))
		}
		return result, nil
	}

	reward, checkinDate, err := c.doCheckin(ctx, headers)
	if err != nil {
		if isCheckinDisabledError(err) {
			result.Enabled = false
			result.Status = "disabled"
			result.Message = "站点未开启签到功能"
			return result, nil
		}
		if strings.Contains(err.Error(), "今日已签到") {
			result.Status = "checked"
			result.Message = "今日已签到"
			return result, nil
		}
		result.Status = "failed"
		result.Message = "签到失败：" + err.Error()
		return result, nil
	}
	result.Status = "signed"
	if strings.TrimSpace(checkinDate) != "" {
		result.Message = "自动签到成功：" + strings.TrimSpace(checkinDate)
	} else {
		result.Message = "自动签到成功"
	}
	result.Reward = ptr(newAPIQuotaToUSD(reward))
	return result, nil
}

type newAPICheckinStatus struct {
	Enabled      bool
	CheckedToday bool
	Records      []newAPICheckinRecord
}

type newAPICheckinRecord struct {
	CheckinDate  string
	QuotaAwarded float64
}

func (c *NewAPIClient) fetchCheckinStatus(ctx context.Context, path string, headers map[string]string) (newAPICheckinStatus, error) {
	var data struct {
		Enabled bool `json:"enabled"`
		Stats   struct {
			CheckedToday bool `json:"checked_in_today"`
			Records      []struct {
				CheckinDate  string `json:"checkin_date"`
				QuotaAwarded any    `json:"quota_awarded"`
			} `json:"records"`
		} `json:"stats"`
	}
	if err := c.request(ctx, http.MethodGet, path, headers, nil, &data); err != nil {
		return newAPICheckinStatus{}, err
	}
	status := newAPICheckinStatus{Enabled: data.Enabled, CheckedToday: data.Stats.CheckedToday}
	for _, record := range data.Stats.Records {
		if quota, ok := nullableFloat(record.QuotaAwarded); ok {
			status.Records = append(status.Records, newAPICheckinRecord{
				CheckinDate:  strings.TrimSpace(record.CheckinDate),
				QuotaAwarded: quota,
			})
		}
	}
	return status, nil
}

func (c *NewAPIClient) doCheckin(ctx context.Context, headers map[string]string) (float64, string, error) {
	var data struct {
		QuotaAwarded any    `json:"quota_awarded"`
		CheckinDate  string `json:"checkin_date"`
	}
	if err := c.request(ctx, http.MethodPost, "api/user/checkin", headers, nil, &data); err != nil {
		return 0, "", err
	}
	reward, _ := nullableFloat(data.QuotaAwarded)
	return reward, data.CheckinDate, nil
}

func checkinRewardForDate(records []newAPICheckinRecord, now time.Time) (float64, bool) {
	today := now.Format("2006-01-02")
	for _, record := range records {
		if strings.TrimSpace(record.CheckinDate) == today {
			return record.QuotaAwarded, true
		}
	}
	return 0, false
}

func isCheckinDisabledError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "签到功能未启用")
}

func (c *NewAPIClient) addTopupHistoryMultiplier(ctx context.Context, headers map[string]string, best **float64) error {
	var page struct {
		Items []struct {
			Amount int64   `json:"amount"`
			Money  float64 `json:"money"`
			Status string  `json:"status"`
		} `json:"items"`
	}
	if err := c.request(ctx, http.MethodGet, "api/user/topup/self?p=1&page_size=20", headers, nil, &page); err != nil {
		return err
	}
	for _, item := range page.Items {
		status := strings.ToLower(strings.TrimSpace(item.Status))
		if status != "success" && status != "completed" && status != "complete" {
			continue
		}
		addRechargeMultiplier(best, float64(item.Amount), item.Money)
	}
	return nil
}

func (c *NewAPIClient) addAmountEndpointMultipliers(ctx context.Context, headers map[string]string, path string, amounts []int64, best **float64) {
	for _, amount := range amounts {
		var data any
		if err := c.request(ctx, http.MethodPost, path, headers, map[string]int64{"amount": amount}, &data); err != nil {
			continue
		}
		if paid, ok := nullableFloat(data); ok {
			addRechargeMultiplier(best, float64(amount), paid)
		}
	}
}

func addCreemProductMultipliers(info map[string]any, best **float64) {
	products, ok := info["creem_products"].([]any)
	if !ok {
		if raw, ok := info["creem_products"].(string); ok && strings.TrimSpace(raw) != "" {
			_ = json.Unmarshal([]byte(raw), &products)
		}
	}
	if len(products) == 0 {
		return
	}
	for _, product := range products {
		entry, ok := product.(map[string]any)
		if !ok {
			continue
		}
		quota, quotaOK := nullableFloat(entry["quota"])
		price, priceOK := nullableFloat(entry["price"])
		if quotaOK && priceOK {
			addRechargeMultiplier(best, quota, price)
		}
	}
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
		key, err := c.getTokenKey(ctx, headers, tokenID, name)
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
	key, err := c.getTokenKey(ctx, headers, tokenID, name)
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

func (c *NewAPIClient) getTokenKey(ctx context.Context, headers map[string]string, tokenID int, name string) (string, error) {
	var result struct {
		Key string `json:"key"`
	}
	if err := c.request(ctx, http.MethodPost, fmt.Sprintf("api/token/%d/key", tokenID), headers, nil, &result); err != nil {
		if key, batchErr := c.getTokenKeyBatch(ctx, headers, tokenID); batchErr == nil {
			return key, nil
		}
		if key, listErr := c.getTokenKeyFromList(ctx, headers, tokenID, name); listErr == nil {
			return key, nil
		}
		return "", fmt.Errorf("get newapi token key: %w", err)
	}
	key, err := normalizeNewAPIKey(result.Key)
	if err == nil {
		return key, nil
	}
	if key, listErr := c.getTokenKeyFromList(ctx, headers, tokenID, name); listErr == nil {
		return key, nil
	}
	return "", err
}

func (c *NewAPIClient) getTokenKeyBatch(ctx context.Context, headers map[string]string, tokenID int) (string, error) {
	var result struct {
		Keys map[string]string `json:"keys"`
	}
	if err := c.request(ctx, http.MethodPost, "api/token/batch/keys", headers, map[string][]int{"ids": []int{tokenID}}, &result); err != nil {
		return "", fmt.Errorf("get newapi token key batch: %w", err)
	}
	if result.Keys == nil {
		return "", fmt.Errorf("newapi token key batch response missing keys")
	}
	key := result.Keys[strconv.Itoa(tokenID)]
	if key == "" {
		key = result.Keys[fmt.Sprintf("%d", tokenID)]
	}
	return normalizeNewAPIKey(key)
}

func (c *NewAPIClient) getTokenKeyFromList(ctx context.Context, headers map[string]string, tokenID int, name string) (string, error) {
	items, err := c.listTokenFirstPage(ctx, headers)
	if err != nil {
		return "", fmt.Errorf("list newapi tokens: %w", err)
	}
	name = strings.TrimSpace(name)
	for _, item := range items {
		if item.ID == tokenID || (name != "" && item.Name == name) {
			return normalizeNewAPIKey(item.Key)
		}
	}
	return "", fmt.Errorf("newapi token %d was not found in token list", tokenID)
}

func normalizeNewAPIKey(key string) (string, error) {
	key = strings.TrimSpace(key)
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
		Items []newAPITokenItem `json:"items"`
	}
	headers := newAPIAuthHeaders(userID, token)
	var searchErr error
	if err := c.request(ctx, http.MethodGet, path, headers, nil, &page); err == nil {
		for _, item := range page.Items {
			if item.Name == name && item.ID > 0 {
				return item.ID, item.Group, true, nil
			}
		}
	} else {
		searchErr = err
	}

	items, listErr := c.listTokenFirstPage(ctx, headers)
	if listErr != nil {
		if searchErr != nil {
			return 0, "", false, fmt.Errorf("search newapi token: %w", searchErr)
		}
		return 0, "", false, nil
	}
	for _, item := range items {
		if item.Name == name && item.ID > 0 {
			return item.ID, item.Group, true, nil
		}
	}
	return 0, "", false, nil
}

func (c *NewAPIClient) listTokenFirstPage(ctx context.Context, headers map[string]string) ([]newAPITokenItem, error) {
	items, err := c.listTokens(ctx, headers, "api/token/?p=1&page_size=100")
	if err == nil {
		return items, nil
	}
	items, sizeErr := c.listTokens(ctx, headers, "api/token/?p=1&size=100")
	if sizeErr == nil {
		return items, nil
	}
	return nil, err
}

func (c *NewAPIClient) listTokens(ctx context.Context, headers map[string]string, path string) ([]newAPITokenItem, error) {
	var page struct {
		Items []newAPITokenItem `json:"items"`
	}
	if err := c.request(ctx, http.MethodGet, path, headers, nil, &page); err != nil {
		return nil, err
	}
	return page.Items, nil
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
			return errors.New(localizeErrorText(envelope.Message))
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
		return nil, localizedHTTPError("NewAPI 上游", endpoint, resp.StatusCode, data)
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
