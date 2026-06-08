package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Sub2APIClient struct {
	baseURL string
	token   string
	auth    sub2APIAuthMode
	client  *http.Client
}

type sub2APIAuthMode string

const (
	sub2APIAuthBearer   sub2APIAuthMode = "bearer"
	sub2APIAuthAdminKey sub2APIAuthMode = "admin_key"

	sub2PlatformOpenAI    = "openai"
	sub2PlatformAnthropic = "anthropic"

	syncedAccountConcurrency = 10
	syncedAccountLoadFactor  = 10
	syncedAccountPriority    = 1

	upstreamManagedKeyPrefix = "pm-"
	upstreamManagedKeyLimit  = 10
)

type sub2Envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type sub2Group struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Platform string  `json:"platform"`
	Status   string  `json:"status"`
	Rate     float64 `json:"rate_multiplier"`
}

type sub2Account struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	Platform    string         `json:"platform"`
	Type        string         `json:"type"`
	Credentials map[string]any `json:"credentials"`
	GroupIDs    []int64        `json:"group_ids"`
	Groups      []sub2Group    `json:"groups"`
	Status      string         `json:"status"`
	Schedulable bool           `json:"schedulable"`
	Concurrency int            `json:"concurrency"`
	Priority    int            `json:"priority"`
	LoadFactor  *int           `json:"load_factor"`
	Rate        *float64       `json:"rate_multiplier"`
}

type sub2AccountTestEvent struct {
	Type    string `json:"type"`
	Success bool   `json:"success,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

type sub2APIKey struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"user_id"`
	Key       string     `json:"key"`
	Name      string     `json:"name"`
	GroupID   *int64     `json:"group_id"`
	Status    string     `json:"status"`
	Group     *sub2Group `json:"group,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func NewSub2APIClient(baseURL string, token string) (*Sub2APIClient, error) {
	return newSub2APIClient(baseURL, token, sub2APIAuthBearer)
}

func NewSub2APIAdminClient(baseURL string, token string) (*Sub2APIClient, error) {
	mode := sub2APIAuthAdminKey
	if looksLikeBearerToken(token) {
		mode = sub2APIAuthBearer
	}
	return newSub2APIClient(baseURL, token, mode)
}

func newSub2APIClient(baseURL string, token string, auth sub2APIAuthMode) (*Sub2APIClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Sub2APIClient{
		baseURL: dockerAwareSub2APIBaseURL(baseURL),
		token:   strings.TrimSpace(token),
		auth:    auth,
		client: &http.Client{
			Timeout: 25 * time.Second,
			Jar:     jar,
		},
	}, nil
}

func looksLikeBearerToken(token string) bool {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		return true
	}
	return strings.Count(token, ".") == 2
}

func dockerAwareSub2APIBaseURL(baseURL string) string {
	normalized := normalizeBaseURL(baseURL)
	if normalized == "" || !runningInDockerContainer() {
		return normalized
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return normalized
	}
	host := strings.ToLower(strings.Trim(parsed.Hostname(), "[]"))
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return normalized
	}
	if port := parsed.Port(); port != "" {
		parsed.Host = net.JoinHostPort("host.docker.internal", port)
	} else {
		parsed.Host = "host.docker.internal"
	}
	return normalizeBaseURL(parsed.String())
}

func runningInDockerContainer() bool {
	if strings.EqualFold(os.Getenv("PRICE_MONITOR_DOCKER_HOST_REWRITE"), "0") {
		return false
	}
	if os.Getenv("PRICE_MONITOR_DOCKER_HOST_REWRITE") != "" {
		return true
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	return false
}

func (c *Sub2APIClient) Login(ctx context.Context, email, password string) error {
	return c.LoginWith2FA(ctx, email, password, "", "")
}

func (c *Sub2APIClient) LoginWith2FA(ctx context.Context, email, password, totpCode, turnstileToken string) error {
	email = strings.TrimSpace(email)
	password = strings.TrimSpace(password)
	if email == "" || password == "" {
		return fmt.Errorf("sub2api access token or email/password is required")
	}
	var out struct {
		AccessToken string `json:"access_token"`
		Requires2FA bool   `json:"requires_2fa"`
		TempToken   string `json:"temp_token"`
	}
	if err := c.request(ctx, http.MethodPost, "api/v1/auth/login", nil, map[string]string{
		"email":           email,
		"password":        password,
		"turnstile_token": strings.TrimSpace(turnstileToken),
	}, &out); err != nil {
		return fmt.Errorf("login sub2api: %w", err)
	}
	if out.Requires2FA {
		totpCode = strings.TrimSpace(totpCode)
		if totpCode == "" {
			return fmt.Errorf("sub2api account requires 2FA code")
		}
		if err := c.request(ctx, http.MethodPost, "api/v1/auth/login/2fa", nil, map[string]string{
			"temp_token": out.TempToken,
			"totp_code":  totpCode,
		}, &out); err != nil {
			return fmt.Errorf("login sub2api 2fa: %w", err)
		}
	}
	c.token = strings.TrimSpace(out.AccessToken)
	if c.token == "" {
		return fmt.Errorf("sub2api login response did not include access_token")
	}
	return nil
}

func (c *Sub2APIClient) AvailableGroups(ctx context.Context) ([]sub2Group, error) {
	var groups []sub2Group
	if err := c.request(ctx, http.MethodGet, "api/v1/groups/available", nil, nil, &groups); err != nil {
		return nil, fmt.Errorf("list sub2api available groups: %w", err)
	}
	return groups, nil
}

func (c *Sub2APIClient) UserGroupRates(ctx context.Context) (map[string]float64, error) {
	raw := map[string]any{}
	if err := c.request(ctx, http.MethodGet, "api/v1/groups/rates", nil, nil, &raw); err != nil {
		return nil, fmt.Errorf("list sub2api user group rates: %w", err)
	}
	rates := make(map[string]float64, len(raw))
	for key, value := range raw {
		if rate, ok := nullableFloat(value); ok {
			rates[strings.TrimSpace(key)] = rate
		}
	}
	return rates, nil
}

func (c *Sub2APIClient) FetchBalance(ctx context.Context) (UpstreamBalance, error) {
	var profile map[string]any
	if err := c.request(ctx, http.MethodGet, "api/v1/user/profile", nil, nil, &profile); err != nil {
		return UpstreamBalance{}, fmt.Errorf("fetch sub2api user balance: %w", err)
	}
	balance, ok := nullableFloat(profile["balance"])
	if !ok {
		return UpstreamBalance{Unit: "usd"}, nil
	}
	return UpstreamBalance{Value: ptr(balance), Unit: "usd"}, nil
}

func (c *Sub2APIClient) FetchRechargeStatus(ctx context.Context) (RechargeStatus, error) {
	var cfg struct {
		Enabled                   bool    `json:"enabled"`
		PaymentEnabled            bool    `json:"payment_enabled"`
		BalanceDisabled           bool    `json:"balance_disabled"`
		BalanceRechargeMultiplier float64 `json:"balance_recharge_multiplier"`
		RechargeFeeRate           float64 `json:"recharge_fee_rate"`
	}
	if err := c.request(ctx, http.MethodGet, "api/v1/payment/config", nil, nil, &cfg); err != nil {
		return RechargeStatus{}, fmt.Errorf("fetch sub2api payment config: %w", err)
	}
	enabled := (cfg.Enabled || cfg.PaymentEnabled) && !cfg.BalanceDisabled
	if !enabled {
		return RechargeStatus{}, nil
	}
	var best *float64
	if cfg.BalanceRechargeMultiplier > 0 {
		paidFactor := 1 + cfg.RechargeFeeRate/100
		if paidFactor <= 0 {
			paidFactor = 1
		}
		addRechargeMultiplier(&best, cfg.BalanceRechargeMultiplier, paidFactor)
	}
	_ = c.addPaymentOrderMultiplier(ctx, &best)
	return RechargeStatus{Enabled: true, Multiplier: best}, nil
}

func (c *Sub2APIClient) EnsureDailyCheckin(ctx context.Context, now time.Time) (CheckinResult, error) {
	checkedAt := now
	if checkedAt.IsZero() {
		checkedAt = time.Now()
	}
	return CheckinResult{
		Enabled:   false,
		Status:    "disabled",
		Unit:      "usd",
		Message:   "不支持签到功能",
		CheckedAt: checkedAt,
	}, nil
}

func (c *Sub2APIClient) addPaymentOrderMultiplier(ctx context.Context, best **float64) error {
	var page struct {
		Items []struct {
			Amount    float64 `json:"amount"`
			PayAmount float64 `json:"pay_amount"`
			Status    string  `json:"status"`
			OrderType string  `json:"order_type"`
		} `json:"items"`
	}
	if err := c.request(ctx, http.MethodGet, "api/v1/payment/orders/my?page=1&page_size=20&order_type=balance", nil, nil, &page); err != nil {
		return err
	}
	for _, item := range page.Items {
		orderType := strings.ToLower(strings.TrimSpace(item.OrderType))
		if orderType != "" && orderType != "balance" {
			continue
		}
		status := strings.ToUpper(strings.TrimSpace(item.Status))
		if status != "COMPLETED" && status != "PAID" && status != "RECHARGING" {
			continue
		}
		addRechargeMultiplier(best, item.Amount, item.PayAmount)
	}
	return nil
}

func (c *Sub2APIClient) EnsureAPIKeyForGroup(ctx context.Context, name string, group sub2Group) (sub2APIKey, string, error) {
	name = strings.TrimSpace(name)
	group.Name = strings.TrimSpace(group.Name)
	if name == "" || group.ID <= 0 {
		return sub2APIKey{}, "", fmt.Errorf("sub2api api key name and group id are required")
	}
	verifiedGroup, err := c.ensureCurrentGroupIdentity(ctx, group)
	if err != nil {
		return sub2APIKey{}, "", err
	}
	group = verifiedGroup
	keys, err := c.listAPIKeys(ctx, "")
	if err != nil {
		return sub2APIKey{}, "", err
	}
	for _, key := range keys {
		if key.GroupID != nil && *key.GroupID == group.ID && strings.TrimSpace(key.Key) != "" {
			_ = c.pruneManagedAPIKeys(ctx, keys, key.ID)
			return normalizeSub2APIKey(key), "reused", nil
		}
	}
	for _, key := range keys {
		if key.Name != name {
			continue
		}
		updated, err := c.updateAPIKeyGroup(ctx, key.ID, name, group.ID)
		if err == nil {
			keys = replaceOrAppendAPIKey(keys, updated)
			_ = c.pruneManagedAPIKeys(ctx, keys, updated.ID)
		}
		return normalizeSub2APIKey(updated), "updated", err
	}
	created, err := c.createAPIKey(ctx, name, group.ID)
	if err == nil {
		keys = append(keys, created)
		_ = c.pruneManagedAPIKeys(ctx, keys, created.ID)
	}
	return normalizeSub2APIKey(created), "created", err
}

func (c *Sub2APIClient) ensureCurrentGroupIdentity(ctx context.Context, expected sub2Group) (sub2Group, error) {
	groups, err := c.AvailableGroups(ctx)
	if err != nil {
		return sub2Group{}, err
	}
	for _, group := range groups {
		if group.ID != expected.ID {
			continue
		}
		if strings.TrimSpace(expected.Name) != "" && !strings.EqualFold(strings.TrimSpace(group.Name), strings.TrimSpace(expected.Name)) {
			return sub2Group{}, fmt.Errorf("sub2api group id %d name changed from %q to %q", expected.ID, expected.Name, group.Name)
		}
		if expected.Rate > 0 {
			currentRate := group.Rate
			userRates, userRatesErr := c.UserGroupRates(ctx)
			if userRatesErr == nil {
				if rate, ok := sub2GroupUserRate(userRates, group); ok {
					currentRate = rate
				}
			}
			if !floatNearlyEqual(currentRate, expected.Rate) {
				return sub2Group{}, fmt.Errorf("sub2api group id %d rate changed from %g to %g", expected.ID, expected.Rate, currentRate)
			}
		}
		return group, nil
	}
	return sub2Group{}, fmt.Errorf("sub2api group id %d was not found", expected.ID)
}

func sub2GroupUserRate(rates map[string]float64, group sub2Group) (float64, bool) {
	if len(rates) == 0 {
		return 0, false
	}
	if rate, ok := rates[strconv.FormatInt(group.ID, 10)]; ok {
		return rate, true
	}
	for key, rate := range rates {
		if strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(group.Name)) {
			return rate, true
		}
	}
	return 0, false
}

func (c *Sub2APIClient) listAPIKeys(ctx context.Context, search string) ([]sub2APIKey, error) {
	const pageSize = 100
	var all []sub2APIKey
	for pageNumber := 1; ; pageNumber++ {
		query := url.Values{}
		query.Set("page", strconv.Itoa(pageNumber))
		query.Set("page_size", strconv.Itoa(pageSize))
		query.Set("sort_by", "created_at")
		query.Set("sort_order", "desc")
		if strings.TrimSpace(search) != "" {
			query.Set("search", strings.TrimSpace(search))
		}
		var page struct {
			Items []sub2APIKey `json:"items"`
			Total int64        `json:"total"`
		}
		if err := c.request(ctx, http.MethodGet, "api/v1/keys?"+query.Encode(), nil, nil, &page); err != nil {
			return nil, fmt.Errorf("list sub2api api keys: %w", err)
		}
		all = append(all, page.Items...)
		if len(page.Items) < pageSize {
			break
		}
		if page.Total > 0 && int64(len(all)) >= page.Total {
			break
		}
	}
	return all, nil
}

func (c *Sub2APIClient) createAPIKey(ctx context.Context, name string, groupID int64) (sub2APIKey, error) {
	var key sub2APIKey
	if err := c.request(ctx, http.MethodPost, "api/v1/keys", nil, map[string]any{
		"name":     name,
		"group_id": groupID,
	}, &key); err != nil {
		return sub2APIKey{}, fmt.Errorf("create sub2api api key: %w", err)
	}
	return key, nil
}

func (c *Sub2APIClient) updateAPIKeyGroup(ctx context.Context, keyID int64, name string, groupID int64) (sub2APIKey, error) {
	if keyID <= 0 {
		return sub2APIKey{}, fmt.Errorf("sub2api api key id is required")
	}
	var key sub2APIKey
	if err := c.request(ctx, http.MethodPut, fmt.Sprintf("api/v1/keys/%d", keyID), nil, map[string]any{
		"name":     name,
		"group_id": groupID,
		"status":   "active",
	}, &key); err != nil {
		return sub2APIKey{}, fmt.Errorf("update sub2api api key %d: %w", keyID, err)
	}
	return key, nil
}

func (c *Sub2APIClient) deleteAPIKey(ctx context.Context, keyID int64) error {
	if keyID <= 0 {
		return fmt.Errorf("sub2api api key id is required")
	}
	if err := c.request(ctx, http.MethodDelete, fmt.Sprintf("api/v1/keys/%d", keyID), nil, nil, nil); err != nil {
		return fmt.Errorf("delete sub2api api key %d: %w", keyID, err)
	}
	return nil
}

func (c *Sub2APIClient) pruneManagedAPIKeys(ctx context.Context, keys []sub2APIKey, keepID int64) error {
	managed := make([]sub2APIKey, 0, len(keys))
	seen := map[int64]bool{}
	for _, key := range keys {
		if key.ID <= 0 || seen[key.ID] || !isManagedUpstreamAPIKeyName(key.Name) {
			continue
		}
		seen[key.ID] = true
		managed = append(managed, key)
	}
	sort.SliceStable(managed, func(i, j int) bool {
		if managed[i].ID == keepID {
			return true
		}
		if managed[j].ID == keepID {
			return false
		}
		leftTime := apiKeySortTime(managed[i])
		rightTime := apiKeySortTime(managed[j])
		if !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		return managed[i].ID > managed[j].ID
	})
	for i := upstreamManagedKeyLimit; i < len(managed); i++ {
		if err := c.deleteAPIKey(ctx, managed[i].ID); err != nil {
			return err
		}
	}
	return nil
}

func replaceOrAppendAPIKey(keys []sub2APIKey, updated sub2APIKey) []sub2APIKey {
	if updated.ID <= 0 {
		return keys
	}
	for i := range keys {
		if keys[i].ID == updated.ID {
			keys[i] = updated
			return keys
		}
	}
	return append(keys, updated)
}

func isManagedUpstreamAPIKeyName(name string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), upstreamManagedKeyPrefix)
}

func apiKeySortTime(key sub2APIKey) time.Time {
	if !key.CreatedAt.IsZero() {
		return key.CreatedAt
	}
	return key.UpdatedAt
}

func floatNearlyEqual(left, right float64) bool {
	return math.Abs(left-right) <= 1e-9
}

func normalizeSub2APIKey(key sub2APIKey) sub2APIKey {
	key.Key = strings.TrimSpace(key.Key)
	if key.Key != "" && !strings.HasPrefix(key.Key, "sk-") {
		key.Key = "sk-" + key.Key
	}
	return key
}

func (c *Sub2APIClient) EnsureGroup(ctx context.Context, groupName string) (sub2Group, error) {
	return c.EnsureGroupByIDOrName(ctx, 0, groupName)
}

func (c *Sub2APIClient) EnsureGroupByIDOrName(ctx context.Context, groupID int64, groupName string) (sub2Group, error) {
	groupName = strings.TrimSpace(groupName)
	if groupID <= 0 && groupName == "" {
		return sub2Group{}, fmt.Errorf("sub2api group name is required")
	}
	groups, err := c.listGroups(ctx)
	if err != nil {
		return sub2Group{}, err
	}
	if groupID > 0 {
		for _, group := range groups {
			if group.ID == groupID {
				return group, nil
			}
		}
		return sub2Group{}, fmt.Errorf("sub2api group id %d was not found", groupID)
	}
	for _, group := range groups {
		if strings.EqualFold(group.Name, groupName) {
			return group, nil
		}
	}
	var group sub2Group
	payload := map[string]any{
		"name":            groupName,
		"description":     "created by newapi price monitor",
		"platform":        "openai",
		"rate_multiplier": 1,
		"status":          "active",
	}
	if err := c.request(ctx, http.MethodPost, "api/v1/admin/groups", nil, payload, &group); err != nil {
		return sub2Group{}, fmt.Errorf("create sub2api group %q: %w", groupName, err)
	}
	return group, nil
}

func (c *Sub2APIClient) UpsertOpenAIAPIKeyAccount(ctx context.Context, accountName, apiBaseURL, apiKey string, group sub2Group) (sub2Account, string, error) {
	return c.UpsertOpenAIAPIKeyAccountGroups(ctx, accountName, apiBaseURL, apiKey, []sub2Group{group})
}

func (c *Sub2APIClient) UpsertOpenAIAPIKeyAccountGroups(ctx context.Context, accountName, apiBaseURL, apiKey string, groups []sub2Group) (sub2Account, string, error) {
	return c.UpsertAPIKeyAccountGroups(ctx, sub2PlatformOpenAI, accountName, apiBaseURL, apiKey, groups)
}

func (c *Sub2APIClient) UpsertAPIKeyAccountGroups(ctx context.Context, platform, accountName, apiBaseURL, apiKey string, groups []sub2Group) (sub2Account, string, error) {
	return c.UpsertAPIKeyAccountGroupsWithRate(ctx, platform, accountName, apiBaseURL, apiKey, groups, nil)
}

func (c *Sub2APIClient) UpsertAPIKeyAccountGroupsWithRate(ctx context.Context, platform, accountName, apiBaseURL, apiKey string, groups []sub2Group, accountRate *float64) (sub2Account, string, error) {
	platform = normalizeSub2Platform(platform)
	apiBaseURL = normalizeBaseURL(apiBaseURL)
	apiKey = strings.TrimSpace(apiKey)
	groups = normalizeSub2Groups(groups)
	accountRate = normalizeSub2AccountRate(accountRate)
	primary := firstSub2Group(groups)
	if accountName == "" {
		accountName = "newapi " + apiBaseURL + " " + primary.Name
	}
	if apiBaseURL == "" || apiKey == "" || len(groups) == 0 {
		return sub2Account{}, "", fmt.Errorf("sub2api account base url, api key and groups are required")
	}
	accounts, err := c.listAPIKeyAccounts(ctx, platform, "", sub2Group{})
	if err != nil {
		return sub2Account{}, "", err
	}
	var sameURL []sub2Account
	for _, account := range accounts {
		if accountMatchesPlatformAPIURL(account, platform, apiBaseURL) {
			sameURL = append(sameURL, account)
		}
	}
	if len(sameURL) > 0 {
		account := preferredSub2Account(sameURL, groups)
		updated, err := c.updateAccountGroupsWithRate(ctx, platform, account, accountName, apiBaseURL, apiKey, groups, accountRate)
		if err != nil {
			return updated, "updated", err
		}
		if err := c.disableDuplicateAPIKeyAccounts(ctx, platform, updated.ID, apiBaseURL, accounts); err != nil {
			return updated, "updated", err
		}
		return updated, "updated", nil
	}
	created, err := c.createAccountGroupsWithRate(ctx, platform, accountName, apiBaseURL, apiKey, groups, accountRate)
	return created, "created", err
}

func (c *Sub2APIClient) ListOpenAIAPIKeyAccounts(ctx context.Context, apiBaseURL string, group sub2Group) ([]sub2Account, error) {
	accounts, err := c.listAccounts(ctx, apiBaseURL, group)
	if err != nil {
		return nil, err
	}
	if apiBaseURL == "" {
		return accounts, nil
	}
	filtered := make([]sub2Account, 0, len(accounts))
	for _, account := range accounts {
		if group.ID <= 0 && group.Name == "" {
			if normalizeBaseURL(stringMapValue(account.Credentials, "base_url")) == normalizeBaseURL(apiBaseURL) {
				filtered = append(filtered, account)
			}
			continue
		}
		if accountMatches(account, apiBaseURL, group) {
			filtered = append(filtered, account)
		}
	}
	return filtered, nil
}

func (c *Sub2APIClient) SetAccountEnabled(ctx context.Context, accountID int64, enabled bool) (sub2Account, error) {
	if accountID <= 0 {
		return sub2Account{}, fmt.Errorf("sub2api account id is required")
	}
	status := "inactive"
	if enabled {
		status = "active"
	}
	var account sub2Account
	if err := c.request(ctx, http.MethodPut, fmt.Sprintf("api/v1/admin/accounts/%d", accountID), nil, map[string]any{
		"status": status,
	}, &account); err != nil {
		return sub2Account{}, fmt.Errorf("set sub2api account status: %w", err)
	}
	if err := c.request(ctx, http.MethodPost, fmt.Sprintf("api/v1/admin/accounts/%d/schedulable", accountID), nil, map[string]bool{
		"schedulable": enabled,
	}, &account); err != nil {
		return sub2Account{}, fmt.Errorf("set sub2api account schedulable: %w", err)
	}
	return account, nil
}

func (c *Sub2APIClient) UpdateAccountAPIKey(ctx context.Context, accountID int64, apiBaseURL, apiKey string, group sub2Group) (sub2Account, error) {
	if accountID <= 0 {
		return sub2Account{}, fmt.Errorf("sub2api account id is required")
	}
	existing, err := c.getAccount(ctx, accountID)
	if err != nil {
		return sub2Account{}, err
	}
	if strings.TrimSpace(apiBaseURL) == "" {
		apiBaseURL = stringMapValue(existing.Credentials, "base_url")
	}
	if group.ID <= 0 {
		if len(existing.Groups) > 0 {
			group = existing.Groups[0]
		} else if len(existing.GroupIDs) > 0 {
			group = sub2Group{ID: existing.GroupIDs[0]}
		}
	}
	return c.updateAccount(ctx, existing, existing.Name, apiBaseURL, apiKey, group)
}

func (c *Sub2APIClient) PrioritizeOpenAIAPIKeyAccountForGroups(ctx context.Context, accountID int64, groups []sub2Group) error {
	return c.PrioritizeOpenAIAPIKeyAccountForGroupsWithRate(ctx, accountID, groups, nil)
}

func (c *Sub2APIClient) PrioritizeOpenAIAPIKeyAccountForGroupsWithRate(ctx context.Context, accountID int64, groups []sub2Group, accountRate *float64) error {
	if accountID <= 0 {
		return fmt.Errorf("sub2api account id is required")
	}
	groups = normalizeSub2Groups(groups)
	if len(groups) == 0 {
		return fmt.Errorf("sub2api account groups are required")
	}
	if err := c.setAccountPriorityWithRate(ctx, accountID, syncedAccountPriority, "active", accountRate); err != nil {
		return err
	}
	if err := c.request(ctx, http.MethodPost, fmt.Sprintf("api/v1/admin/accounts/%d/schedulable", accountID), nil, map[string]bool{"schedulable": true}, nil); err != nil {
		return fmt.Errorf("set sub2api account %d schedulable: %w", accountID, err)
	}
	return nil
}

func (c *Sub2APIClient) TestAccountConnection(ctx context.Context, accountID int64, modelName string) error {
	if accountID <= 0 {
		return fmt.Errorf("sub2api account id is required")
	}
	modelName = strings.TrimSpace(modelName)
	payload := map[string]string{
		"model_id": modelName,
		"prompt":   "hi",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint, err := url.JoinPath(c.baseURL, fmt.Sprintf("api/v1/admin/accounts/%d/test", accountID))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", newAPIUserAgent)
	if c.token != "" {
		token := strings.TrimSpace(c.token)
		switch c.auth {
		case sub2APIAuthAdminKey:
			req.Header.Set("x-api-key", token)
		default:
			req.Header.Set("Authorization", "Bearer "+trimBearerPrefix(token))
		}
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("sub2api %s returned HTTP 401: %s; main sub2api admin auth failed", endpoint, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("sub2api %s returned HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := parseSub2AccountTestSSE(string(body)); err != nil {
		return err
	}
	return nil
}

func parseSub2AccountTestSSE(body string) error {
	var sawCompletion bool
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if raw == "" || raw == "[DONE]" {
			continue
		}
		var event sub2AccountTestEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			continue
		}
		if strings.EqualFold(event.Type, "error") {
			return errors.New(firstNonEmpty(event.Error, event.Message, raw))
		}
		if strings.EqualFold(event.Type, "test_complete") && event.Success {
			sawCompletion = true
		}
	}
	if !sawCompletion {
		return fmt.Errorf("sub2api account test did not report success")
	}
	return nil
}

func (c *Sub2APIClient) DisableOtherAPIKeyAccountsForGroups(ctx context.Context, platform string, keepID int64, groups []sub2Group) error {
	if keepID <= 0 {
		return fmt.Errorf("sub2api account id is required")
	}
	platform = normalizeSub2Platform(platform)
	groups = normalizeSub2Groups(groups)
	disabled := map[int64]struct{}{}
	for _, group := range groups {
		accounts, err := c.listAccountsFiltered(ctx, platform, "", group, "apikey")
		if err != nil {
			return err
		}
		for _, account := range accounts {
			if account.ID <= 0 || account.ID == keepID {
				continue
			}
			if _, ok := disabled[account.ID]; ok {
				continue
			}
			if _, err := c.SetAccountEnabled(ctx, account.ID, false); err != nil {
				return fmt.Errorf("disable sub2api account %d in group %s: %w", account.ID, group.Name, err)
			}
			disabled[account.ID] = struct{}{}
		}
	}
	return nil
}

func (c *Sub2APIClient) getAccount(ctx context.Context, accountID int64) (sub2Account, error) {
	var account sub2Account
	if err := c.request(ctx, http.MethodGet, fmt.Sprintf("api/v1/admin/accounts/%d", accountID), nil, nil, &account); err != nil {
		return sub2Account{}, fmt.Errorf("get sub2api account %d: %w", accountID, err)
	}
	return account, nil
}

func (c *Sub2APIClient) listGroups(ctx context.Context) ([]sub2Group, error) {
	var groups []sub2Group
	if err := c.request(ctx, http.MethodGet, "api/v1/admin/groups/all", nil, nil, &groups); err != nil {
		return nil, fmt.Errorf("list sub2api groups: %w", err)
	}
	return groups, nil
}

func (c *Sub2APIClient) listAccounts(ctx context.Context, apiBaseURL string, group sub2Group) ([]sub2Account, error) {
	return c.listAPIKeyAccounts(ctx, sub2PlatformOpenAI, apiBaseURL, group)
}

func (c *Sub2APIClient) listAPIKeyAccounts(ctx context.Context, platform, apiBaseURL string, group sub2Group) ([]sub2Account, error) {
	return c.listAccountsFiltered(ctx, normalizeSub2Platform(platform), apiBaseURL, group, "apikey")
}

func (c *Sub2APIClient) listOpenAIAccounts(ctx context.Context, group sub2Group) ([]sub2Account, error) {
	return c.listAccountsFiltered(ctx, sub2PlatformOpenAI, "", group, "")
}

func (c *Sub2APIClient) listAccountsFiltered(ctx context.Context, platform, apiBaseURL string, group sub2Group, accountType string) ([]sub2Account, error) {
	const pageSize = 100
	platform = normalizeSub2Platform(platform)
	var all []sub2Account
	for pageNumber := 1; ; pageNumber++ {
		query := url.Values{}
		query.Set("platform", platform)
		if strings.TrimSpace(accountType) != "" {
			query.Set("type", strings.TrimSpace(accountType))
		}
		query.Set("page", strconv.Itoa(pageNumber))
		query.Set("page_size", strconv.Itoa(pageSize))
		query.Set("sort_by", "created_at")
		query.Set("sort_order", "desc")
		if group.ID > 0 {
			query.Set("group", strconv.FormatInt(group.ID, 10))
		}
		if apiBaseURL != "" {
			query.Set("search", strings.TrimRight(apiBaseURL, "/"))
		}
		var page struct {
			Items []sub2Account `json:"items"`
			Total int64         `json:"total"`
		}
		if err := c.request(ctx, http.MethodGet, "api/v1/admin/accounts?"+query.Encode(), nil, nil, &page); err != nil {
			return nil, fmt.Errorf("list sub2api accounts: %w", err)
		}
		all = append(all, page.Items...)
		if len(page.Items) < pageSize {
			break
		}
		if page.Total > 0 && int64(len(all)) >= page.Total {
			break
		}
	}
	return all, nil
}

func (c *Sub2APIClient) createAccount(ctx context.Context, name, apiBaseURL, apiKey string, group sub2Group) (sub2Account, error) {
	return c.createAccountGroups(ctx, sub2PlatformOpenAI, name, apiBaseURL, apiKey, []sub2Group{group})
}

func (c *Sub2APIClient) createAccountGroups(ctx context.Context, platform, name, apiBaseURL, apiKey string, groups []sub2Group) (sub2Account, error) {
	return c.createAccountGroupsWithRate(ctx, platform, name, apiBaseURL, apiKey, groups, nil)
}

func (c *Sub2APIClient) createAccountGroupsWithRate(ctx context.Context, platform, name, apiBaseURL, apiKey string, groups []sub2Group, accountRate *float64) (sub2Account, error) {
	var account sub2Account
	platform = normalizeSub2Platform(platform)
	accountRate = normalizeSub2AccountRate(accountRate)
	confirm := true
	payload := map[string]any{
		"name":                       name,
		"platform":                   platform,
		"type":                       "apikey",
		"credentials":                openAICredentials(apiBaseURL, apiKey),
		"concurrency":                syncedAccountConcurrency,
		"priority":                   syncedAccountPriority,
		"load_factor":                syncedAccountLoadFactor,
		"group_ids":                  sub2GroupIDs(groups),
		"status":                     "active",
		"schedulable":                true,
		"confirm_mixed_channel_risk": confirm,
	}
	if accountRate != nil {
		payload["rate_multiplier"] = *accountRate
	}
	if err := c.request(ctx, http.MethodPost, "api/v1/admin/accounts", nil, payload, &account); err != nil {
		return sub2Account{}, fmt.Errorf("create sub2api account: %w", err)
	}
	if account.ID > 0 {
		_ = c.request(ctx, http.MethodPost, fmt.Sprintf("api/v1/admin/accounts/%d/schedulable", account.ID), nil, map[string]bool{"schedulable": true}, nil)
	}
	return account, nil
}

func (c *Sub2APIClient) updateAccount(ctx context.Context, account sub2Account, name, apiBaseURL, apiKey string, group sub2Group) (sub2Account, error) {
	return c.updateAccountGroups(ctx, account.Platform, account, name, apiBaseURL, apiKey, []sub2Group{group})
}

func (c *Sub2APIClient) updateAccountGroups(ctx context.Context, platform string, account sub2Account, name, apiBaseURL, apiKey string, groups []sub2Group) (sub2Account, error) {
	return c.updateAccountGroupsWithRate(ctx, platform, account, name, apiBaseURL, apiKey, groups, nil)
}

func (c *Sub2APIClient) updateAccountGroupsWithRate(ctx context.Context, platform string, account sub2Account, name, apiBaseURL, apiKey string, groups []sub2Group, accountRate *float64) (sub2Account, error) {
	var updated sub2Account
	platform = normalizeSub2Platform(firstNonEmpty(platform, account.Platform))
	accountRate = normalizeSub2AccountRate(accountRate)
	groupIDs := sub2GroupIDs(groups)
	priority := syncedAccountPriority
	confirm := true
	payload := map[string]any{
		"name":                       firstNonEmpty(name, account.Name),
		"type":                       "apikey",
		"credentials":                openAICredentials(apiBaseURL, apiKey),
		"priority":                   &priority,
		"status":                     "active",
		"schedulable":                true,
		"group_ids":                  &groupIDs,
		"confirm_mixed_channel_risk": confirm,
	}
	if accountRate != nil {
		payload["rate_multiplier"] = *accountRate
	}
	if err := c.request(ctx, http.MethodPut, fmt.Sprintf("api/v1/admin/accounts/%d", account.ID), nil, payload, &updated); err != nil {
		return sub2Account{}, fmt.Errorf("update sub2api account %d: %w", account.ID, err)
	}
	_ = c.request(ctx, http.MethodPost, fmt.Sprintf("api/v1/admin/accounts/%d/schedulable", account.ID), nil, map[string]bool{"schedulable": true}, nil)
	return updated, nil
}

func (c *Sub2APIClient) setAccountPriorityWithRate(ctx context.Context, accountID int64, priority int, status string, accountRate *float64) error {
	if accountID <= 0 {
		return fmt.Errorf("sub2api account id is required")
	}
	accountRate = normalizeSub2AccountRate(accountRate)
	payload := map[string]any{
		"priority": priority,
	}
	if strings.TrimSpace(status) != "" {
		payload["status"] = strings.TrimSpace(status)
	}
	if accountRate != nil {
		payload["rate_multiplier"] = *accountRate
	}
	if err := c.request(ctx, http.MethodPut, fmt.Sprintf("api/v1/admin/accounts/%d", accountID), nil, payload, nil); err != nil {
		return fmt.Errorf("set sub2api account %d priority: %w", accountID, err)
	}
	return nil
}

func (c *Sub2APIClient) disableDuplicateAPIKeyAccounts(ctx context.Context, platform string, keepID int64, apiBaseURL string, accounts []sub2Account) error {
	platform = normalizeSub2Platform(platform)
	for _, account := range accounts {
		if account.ID <= 0 || account.ID == keepID || !accountMatchesPlatformAPIURL(account, platform, apiBaseURL) {
			continue
		}
		if err := c.setAccountPriorityWithRate(ctx, account.ID, 100, "inactive", nil); err != nil {
			return err
		}
		if err := c.request(ctx, http.MethodPost, fmt.Sprintf("api/v1/admin/accounts/%d/schedulable", account.ID), nil, map[string]bool{"schedulable": false}, nil); err != nil {
			return fmt.Errorf("disable duplicate sub2api account %d schedulable: %w", account.ID, err)
		}
	}
	return nil
}

func preferredSub2Account(accounts []sub2Account, groups []sub2Group) sub2Account {
	if len(accounts) == 0 {
		return sub2Account{}
	}
	targetIDs := sub2GroupIDs(groups)
	sort.SliceStable(accounts, func(i, j int) bool {
		leftExactGroups := sub2AccountGroupIDsEqual(accounts[i], targetIDs)
		rightExactGroups := sub2AccountGroupIDsEqual(accounts[j], targetIDs)
		if leftExactGroups != rightExactGroups {
			return leftExactGroups
		}
		leftActive := accounts[i].Status == "active" && accounts[i].Schedulable
		rightActive := accounts[j].Status == "active" && accounts[j].Schedulable
		if leftActive != rightActive {
			return leftActive
		}
		return accounts[i].ID < accounts[j].ID
	})
	return accounts[0]
}

func openAICredentials(apiBaseURL, apiKey string) map[string]any {
	return map[string]any{
		"base_url": strings.TrimRight(normalizeBaseURL(apiBaseURL), "/"),
		"api_key":  apiKey,
	}
}

func accountMatches(account sub2Account, apiBaseURL string, group sub2Group) bool {
	if !accountMatchesPlatform(account, sub2PlatformOpenAI) {
		return false
	}
	if !accountMatchesAPIURL(account, apiBaseURL) {
		return false
	}
	if group.ID > 0 {
		for _, id := range account.GroupIDs {
			if id == group.ID {
				return true
			}
		}
		for _, g := range account.Groups {
			if g.ID == group.ID {
				return true
			}
		}
	}
	return group.Name != "" && strings.Contains(strings.ToLower(account.Name), strings.ToLower(group.Name))
}

func accountMatchesAPIURL(account sub2Account, apiBaseURL string) bool {
	if strings.TrimSpace(account.Platform) == "" || account.Type != "apikey" {
		return false
	}
	baseURL := normalizeBaseURL(stringMapValue(account.Credentials, "base_url"))
	target := normalizeBaseURL(apiBaseURL)
	if baseURL == "" || target == "" {
		return false
	}
	if baseURL == target {
		return true
	}
	return normalizedURLHostname(baseURL) != "" && normalizedURLHostname(baseURL) == normalizedURLHostname(target)
}

func accountMatchesAPIURLAndGroups(account sub2Account, apiBaseURL string, groups []sub2Group) bool {
	if !accountMatchesAPIURL(account, apiBaseURL) {
		return false
	}
	return sub2AccountGroupIDsEqual(account, sub2GroupIDs(groups))
}

func accountMatchesPlatformAPIURL(account sub2Account, platform string, apiBaseURL string) bool {
	return accountMatchesPlatform(account, platform) && accountMatchesAPIURL(account, apiBaseURL)
}

func accountMatchesPlatform(account sub2Account, platform string) bool {
	return strings.EqualFold(strings.TrimSpace(account.Platform), normalizeSub2Platform(platform)) && account.Type == "apikey"
}

func normalizeSub2Platform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "claud", "claude", "anthropic":
		return sub2PlatformAnthropic
	case "", "codex", "gpt", "openai":
		return sub2PlatformOpenAI
	default:
		return strings.ToLower(strings.TrimSpace(platform))
	}
}

func normalizedURLHostname(value string) string {
	parsed, err := url.Parse(normalizeBaseURL(value))
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.Trim(parsed.Hostname(), "[]"))
	return strings.TrimPrefix(host, "www.")
}

func sub2AccountGroupIDsEqual(account sub2Account, targetIDs []int64) bool {
	actual := make([]int64, 0, len(account.GroupIDs)+len(account.Groups))
	seen := map[int64]bool{}
	for _, id := range account.GroupIDs {
		if id > 0 && !seen[id] {
			seen[id] = true
			actual = append(actual, id)
		}
	}
	for _, group := range account.Groups {
		if group.ID > 0 && !seen[group.ID] {
			seen[group.ID] = true
			actual = append(actual, group.ID)
		}
	}
	target := make([]int64, 0, len(targetIDs))
	seen = map[int64]bool{}
	for _, id := range targetIDs {
		if id > 0 && !seen[id] {
			seen[id] = true
			target = append(target, id)
		}
	}
	sort.Slice(actual, func(i, j int) bool { return actual[i] < actual[j] })
	sort.Slice(target, func(i, j int) bool { return target[i] < target[j] })
	if len(actual) != len(target) {
		return false
	}
	for i := range actual {
		if actual[i] != target[i] {
			return false
		}
	}
	return len(target) > 0
}

func normalizeSub2Groups(groups []sub2Group) []sub2Group {
	out := make([]sub2Group, 0, len(groups))
	seen := map[int64]bool{}
	for _, group := range groups {
		if group.ID <= 0 || seen[group.ID] {
			continue
		}
		seen[group.ID] = true
		out = append(out, group)
	}
	return out
}

func normalizeSub2AccountRate(rate *float64) *float64 {
	if rate == nil || math.IsNaN(*rate) || math.IsInf(*rate, 0) || *rate < 0 {
		return nil
	}
	normalized := *rate
	return &normalized
}

func firstSub2Group(groups []sub2Group) sub2Group {
	if len(groups) == 0 {
		return sub2Group{}
	}
	return groups[0]
}

func sub2GroupIDs(groups []sub2Group) []int64 {
	groups = normalizeSub2Groups(groups)
	ids := make([]int64, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	return ids
}

func stringMapValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	if value, ok := values[key].(string); ok {
		return value
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (c *Sub2APIClient) request(ctx context.Context, method, path string, headers map[string]string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	endpoint, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return err
	}
	if strings.Contains(path, "?") {
		endpoint = strings.TrimRight(c.baseURL, "/") + "/" + strings.TrimLeft(path, "/")
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", newAPIUserAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		token := strings.TrimSpace(c.token)
		switch c.auth {
		case sub2APIAuthAdminKey:
			req.Header.Set("x-api-key", token)
		default:
			req.Header.Set("Authorization", "Bearer "+trimBearerPrefix(token))
		}
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized && strings.Contains(path, "/admin/") {
			return fmt.Errorf("sub2api %s returned HTTP 401: %s; main sub2api admin auth failed, use the admin API key generated in sub2api admin settings (sent as x-api-key), or paste a valid admin JWT as Bearer token", endpoint, strings.TrimSpace(string(data)))
		}
		return fmt.Errorf("sub2api %s returned HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var envelope sub2Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decode sub2api response: %w", err)
	}
	if envelope.Code != 0 {
		if envelope.Message != "" {
			return errors.New(envelope.Message)
		}
		return fmt.Errorf("sub2api returned code %d", envelope.Code)
	}
	if out != nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("decode sub2api data: %w", err)
		}
	}
	return nil
}

func trimBearerPrefix(token string) string {
	if len(token) >= len("Bearer ") && strings.EqualFold(token[:len("Bearer ")], "Bearer ") {
		return strings.TrimSpace(token[len("Bearer "):])
	}
	return token
}
