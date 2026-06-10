package app

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultOfficialPriceURL = "https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/main/model_prices_and_context_window.json"
	pricePerMillion         = 1_000_000
)

//go:embed resources/model-pricing/model_prices_and_context_window.json
var embeddedOfficialPriceJSON []byte

type sub2APIUserPriceInput struct {
	Sub2APIUpstreamID int64  `json:"sub2api_upstream_id"`
	BaseURL           string `json:"base_url"`
	Email             string `json:"email"`
	Password          string `json:"password"`
	AuthToken         string `json:"auth_token"`
	TOTPCode          string `json:"totp_code"`
	TurnstileToken    string `json:"turnstile_token"`
	ModelKeyword      string `json:"model_keyword"`
	Models            string `json:"models"`
	Providers         string `json:"providers"`
	Modes             string `json:"modes"`
	Groups            string `json:"groups"`
	Platforms         string `json:"platforms"`
	PriceURL          string `json:"price_url"`
	Limit             int    `json:"limit"`
}

type sub2APIUserPriceResult struct {
	PriceSource    string                 `json:"price_source"`
	Groups         []sub2Group            `json:"groups"`
	UserGroupRates map[string]float64     `json:"user_group_rates"`
	CheapestGroups []sub2CheapestGroup    `json:"cheapest_groups"`
	Rows           []Sub2APIUserPriceRow  `json:"rows"`
	TotalRows      int                    `json:"total_rows"`
	Filters        map[string]interface{} `json:"filters"`
}

type sub2APIUserInspectResult struct {
	Groups         []sub2Group         `json:"groups"`
	UserGroupRates map[string]float64  `json:"user_group_rates"`
	CheapestGroups []sub2CheapestGroup `json:"cheapest_groups"`
	Filters        map[string]string   `json:"filters"`
}

type sub2APIUserFilterOptionsResult struct {
	PriceSource string                 `json:"price_source"`
	Groups      []sub2Group            `json:"groups"`
	Platforms   []sub2FilterOption     `json:"platforms"`
	Providers   []sub2FilterOption     `json:"providers"`
	Modes       []sub2FilterOption     `json:"modes"`
	Models      []sub2FilterOption     `json:"models"`
	Summary     map[string]interface{} `json:"summary"`
}

type sub2FilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int    `json:"count,omitempty"`
}

type sub2CheapestGroup struct {
	Platform         string   `json:"platform"`
	PlatformLabel    string   `json:"platform_label"`
	GroupID          int64    `json:"group_id"`
	GroupName        string   `json:"group_name"`
	GroupStatus      string   `json:"group_status"`
	GroupDefaultRate *float64 `json:"group_default_rate"`
	UserGroupRate    *float64 `json:"user_group_rate"`
	EffectiveRate    float64  `json:"effective_rate"`
}

func (s *Server) inspectSub2APIUser(ctx context.Context, input sub2APIUserPriceInput) (sub2APIUserInspectResult, error) {
	groups, userRates, err := s.fetchSub2APIUserGroups(ctx, input)
	if err != nil {
		return sub2APIUserInspectResult{}, err
	}
	return sub2APIUserInspectResult{
		Groups:         groups,
		UserGroupRates: userRates,
		CheapestGroups: cheapestSub2GroupsByPlatform(groups, userRates, input.Platforms),
		Filters: map[string]string{
			"platforms": input.Platforms,
		},
	}, nil
}

func (s *Server) fetchSub2APIUserFilterOptions(ctx context.Context, input sub2APIUserPriceInput) (sub2APIUserFilterOptionsResult, error) {
	groups, _, err := s.fetchSub2APIUserGroups(ctx, input)
	if err != nil {
		return sub2APIUserFilterOptionsResult{}, err
	}
	priceURL := firstNonEmpty(input.PriceURL, defaultOfficialPriceURL)
	officialPrices, priceSource, err := loadOfficialPrices(ctx, priceURL)
	if err != nil {
		return sub2APIUserFilterOptionsResult{}, err
	}
	return sub2APIUserFilterOptionsResult{
		PriceSource: priceSource,
		Groups:      groups,
		Platforms:   sub2PlatformOptions(groups),
		Providers:   officialStringOptions(officialPrices, "litellm_provider", 80),
		Modes:       officialStringOptions(officialPrices, "mode", 40),
		Models:      officialModelOptions(officialPrices, input.ModelKeyword, 300),
		Summary: map[string]interface{}{
			"group_count": len(groups),
			"model_count": len(officialPrices),
		},
	}, nil
}

func (s *Server) fetchSub2APIUserPrices(ctx context.Context, input sub2APIUserPriceInput) (sub2APIUserPriceResult, error) {
	_, groups, userRates, err := s.fetchSub2APIUserClientGroups(ctx, input)
	if err != nil {
		return sub2APIUserPriceResult{}, err
	}
	priceURL := firstNonEmpty(input.PriceURL, defaultOfficialPriceURL)
	officialPrices, priceSource, err := loadOfficialPrices(ctx, priceURL)
	if err != nil {
		return sub2APIUserPriceResult{}, err
	}
	rows := buildSub2APIUserPriceRows(officialPrices, groups, userRates, input)
	totalRows := len(rows)
	limit := input.Limit
	if limit <= 0 {
		limit = 500
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return sub2APIUserPriceResult{
		PriceSource:    priceSource,
		Groups:         groups,
		UserGroupRates: userRates,
		CheapestGroups: cheapestSub2GroupsByPlatform(groups, userRates, input.Platforms),
		Rows:           rows,
		TotalRows:      totalRows,
		Filters: map[string]interface{}{
			"model_keyword": input.ModelKeyword,
			"models":        input.Models,
			"providers":     input.Providers,
			"modes":         firstNonEmpty(input.Modes, "chat,responses,image_generation"),
			"groups":        input.Groups,
			"platforms":     input.Platforms,
			"limit":         limit,
		},
	}, nil
}

func (s *Server) fetchSub2APIUserGroups(ctx context.Context, input sub2APIUserPriceInput) ([]sub2Group, map[string]float64, error) {
	_, groups, userRates, err := s.fetchSub2APIUserClientGroups(ctx, input)
	return groups, userRates, err
}

func (s *Server) fetchSub2APIUserClientGroups(ctx context.Context, input sub2APIUserPriceInput) (*Sub2APIClient, []sub2Group, map[string]float64, error) {
	cfg, err := s.sub2APIUserSourceConfig(ctx, input)
	if err != nil {
		return nil, nil, nil, err
	}
	return s.fetchSub2APIUserClientGroupsForSource(ctx, cfg)
}

func (s *Server) fetchSub2APIUserClientGroupsForSource(ctx context.Context, cfg sub2APIUserSourceConfig) (*Sub2APIClient, []sub2Group, map[string]float64, error) {
	client, err := s.sub2APIClientForUserSource(ctx, cfg, false)
	if err != nil {
		return nil, nil, nil, err
	}
	groups, err := client.AvailableGroups(ctx)
	if err != nil {
		if isSessionAuthError(err) {
			client, err = s.sub2APIClientForUserSource(ctx, cfg, true)
			if err == nil {
				groups, err = client.AvailableGroups(ctx)
			}
		}
		if err != nil {
			s.saveSub2APIUserSession(ctx, cfg, client, err.Error())
			return nil, nil, nil, err
		}
	}
	userRates, err := client.UserGroupRates(ctx)
	if err != nil {
		if isSessionAuthError(err) {
			client, err = s.sub2APIClientForUserSource(ctx, cfg, true)
			if err == nil {
				groups, err = client.AvailableGroups(ctx)
			}
			if err == nil {
				userRates, err = client.UserGroupRates(ctx)
			}
		}
		if err != nil {
			s.saveSub2APIUserSession(ctx, cfg, client, err.Error())
			return nil, nil, nil, err
		}
	}
	s.saveSub2APIUserSession(ctx, cfg, client, "")
	return client, groups, userRates, nil
}

func loadOfficialPrices(ctx context.Context, priceURL string) (map[string]any, string, error) {
	prices, err := fetchOfficialPrices(ctx, priceURL)
	if err == nil {
		return prices, priceURL, nil
	}
	fallback, fallbackErr := decodeOfficialPrices(embeddedOfficialPriceJSON)
	if fallbackErr != nil {
		return nil, "", fmt.Errorf("%w; embedded fallback failed: %v", err, fallbackErr)
	}
	return fallback, "embedded fallback: model_prices_and_context_window.json", nil
}

func fetchOfficialPrices(ctx context.Context, priceURL string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, priceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "newapi-price-monitor/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch official model prices: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, localizedHTTPError("官方价格接口", priceURL, resp.StatusCode, data)
	}
	return decodeOfficialPrices(data)
}

func decodeOfficialPrices(data []byte) (map[string]any, error) {
	var prices map[string]any
	if err := json.Unmarshal(data, &prices); err != nil {
		return nil, fmt.Errorf("decode official model prices: %w", err)
	}
	return prices, nil
}

func buildSub2APIUserPriceRows(officialPrices map[string]any, groups []sub2Group, userRates map[string]float64, input sub2APIUserPriceInput) []Sub2APIUserPriceRow {
	modelFilter := parseCSVFilter(input.Models)
	providerFilter := parseCSVFilter(input.Providers)
	modeFilter := parseCSVFilter(firstNonEmpty(input.Modes, "chat,responses,image_generation"))
	groupFilter := parseCSVFilter(input.Groups)
	platformFilter := parseCSVFilter(input.Platforms)
	keyword := strings.ToLower(strings.TrimSpace(input.ModelKeyword))

	selectedGroups := make([]sub2Group, 0, len(groups))
	for _, group := range groups {
		if sub2GroupMatches(group, groupFilter, platformFilter) {
			selectedGroups = append(selectedGroups, group)
		}
	}

	modelNames := make([]string, 0, len(officialPrices))
	for modelName := range officialPrices {
		modelNames = append(modelNames, modelName)
	}
	sort.Strings(modelNames)

	rows := make([]Sub2APIUserPriceRow, 0)
	for _, modelName := range modelNames {
		entry := asMap(officialPrices[modelName])
		if len(entry) == 0 {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(modelName), keyword) {
			continue
		}
		if !sub2ModelMatches(modelName, entry, modelFilter, providerFilter, modeFilter) {
			continue
		}
		inputPrice := officialPrice(entry, "input_cost_per_token")
		outputPrice := officialPrice(entry, "output_cost_per_token")
		cacheWritePrice := officialPrice(entry, "cache_creation_input_token_cost")
		cacheWrite1hPrice := officialPrice(entry, "cache_creation_input_token_cost_above_1hr")
		cacheReadPrice := officialPrice(entry, "cache_read_input_token_cost")
		for _, group := range selectedGroups {
			rate, userRatePtr, defaultRatePtr := effectiveSub2GroupRate(group, userRates)
			rows = append(rows, Sub2APIUserPriceRow{
				ModelName:                      modelName,
				Provider:                       stringValue(entry["litellm_provider"]),
				Mode:                           stringValue(entry["mode"]),
				GroupID:                        group.ID,
				GroupName:                      group.Name,
				GroupPlatform:                  group.Platform,
				GroupDefaultRate:               defaultRatePtr,
				UserGroupRate:                  userRatePtr,
				EffectiveRate:                  rate,
				OfficialInputPerMillion:        perMillionPtr(inputPrice),
				OfficialOutputPerMillion:       perMillionPtr(outputPrice),
				OfficialCacheWritePerMillion:   perMillionPtr(cacheWritePrice),
				OfficialCacheWrite1hPerMillion: perMillionPtr(cacheWrite1hPrice),
				OfficialCacheReadPerMillion:    perMillionPtr(cacheReadPrice),
				FinalInputPerMillion:           multiplyPerMillionPtr(inputPrice, rate),
				FinalOutputPerMillion:          multiplyPerMillionPtr(outputPrice, rate),
				FinalCacheWritePerMillion:      multiplyPerMillionPtr(cacheWritePrice, rate),
				FinalCacheWrite1hPerMillion:    multiplyPerMillionPtr(cacheWrite1hPrice, rate),
				FinalCacheReadPerMillion:       multiplyPerMillionPtr(cacheReadPrice, rate),
				MaxInputTokens:                 entry["max_input_tokens"],
				MaxOutputTokens:                entry["max_output_tokens"],
			})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ModelName != rows[j].ModelName {
			return rows[i].ModelName < rows[j].ModelName
		}
		left := nullablePriceValue(rows[i].FinalInputPerMillion, rows[i].FinalOutputPerMillion)
		right := nullablePriceValue(rows[j].FinalInputPerMillion, rows[j].FinalOutputPerMillion)
		if left != right {
			return left < right
		}
		return rows[i].GroupName < rows[j].GroupName
	})
	return rows
}

func sub2PlatformOptions(groups []sub2Group) []sub2FilterOption {
	counts := map[string]int{}
	for _, group := range groups {
		platform := strings.ToLower(strings.TrimSpace(group.Platform))
		if platform == "" {
			platform = "unknown"
		}
		counts[platform]++
	}
	values := make([]string, 0, len(counts))
	for value := range counts {
		values = append(values, value)
	}
	sort.Strings(values)
	out := make([]sub2FilterOption, 0, len(values))
	for _, value := range values {
		out = append(out, sub2FilterOption{Value: value, Label: sub2PlatformLabel(value), Count: counts[value]})
	}
	return out
}

func officialStringOptions(officialPrices map[string]any, key string, limit int) []sub2FilterOption {
	counts := map[string]int{}
	labels := map[string]string{}
	for _, raw := range officialPrices {
		entry := asMap(raw)
		value := strings.TrimSpace(stringValue(entry[key]))
		if value == "" {
			continue
		}
		normalized := strings.ToLower(value)
		counts[normalized]++
		if labels[normalized] == "" {
			labels[normalized] = value
		}
	}
	values := make([]string, 0, len(counts))
	for value := range counts {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if counts[values[i]] != counts[values[j]] {
			return counts[values[i]] > counts[values[j]]
		}
		return values[i] < values[j]
	})
	if limit > 0 && len(values) > limit {
		values = values[:limit]
	}
	out := make([]sub2FilterOption, 0, len(values))
	for _, value := range values {
		out = append(out, sub2FilterOption{Value: labels[value], Label: labels[value], Count: counts[value]})
	}
	return out
}

func officialModelOptions(officialPrices map[string]any, keyword string, limit int) []sub2FilterOption {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	models := make([]string, 0, len(officialPrices))
	for modelName := range officialPrices {
		if keyword != "" && !strings.Contains(strings.ToLower(modelName), keyword) {
			continue
		}
		models = append(models, modelName)
	}
	sort.Strings(models)
	if limit > 0 && len(models) > limit {
		models = models[:limit]
	}
	out := make([]sub2FilterOption, 0, len(models))
	for _, modelName := range models {
		out = append(out, sub2FilterOption{Value: modelName, Label: modelName})
	}
	return out
}

func parseCSVFilter(value string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, item := range strings.Split(value, ",") {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			out[item] = struct{}{}
		}
	}
	return out
}

func sub2ModelMatches(modelName string, entry map[string]any, models, providers, modes map[string]struct{}) bool {
	if len(models) > 0 {
		if _, ok := models[strings.ToLower(modelName)]; !ok {
			return false
		}
	}
	if len(providers) > 0 {
		if _, ok := providers[strings.ToLower(stringValue(entry["litellm_provider"]))]; !ok {
			return false
		}
	}
	if len(modes) > 0 {
		if _, ok := modes[strings.ToLower(stringValue(entry["mode"]))]; !ok {
			return false
		}
	}
	return true
}

func sub2GroupMatches(group sub2Group, groups, platforms map[string]struct{}) bool {
	if len(groups) > 0 {
		_, byID := groups[strconv.FormatInt(group.ID, 10)]
		_, byName := groups[strings.ToLower(group.Name)]
		if !byID && !byName {
			return false
		}
	}
	if len(platforms) > 0 {
		if _, ok := platforms[strings.ToLower(group.Platform)]; !ok {
			return false
		}
	}
	return true
}

func effectiveSub2GroupRate(group sub2Group, userRates map[string]float64) (float64, *float64, *float64) {
	defaultRate := group.Rate
	if defaultRate == 0 {
		defaultRate = 1
	}
	defaultRatePtr := ptr(defaultRate)
	if rate, ok := userRates[strconv.FormatInt(group.ID, 10)]; ok {
		return rate, ptr(rate), defaultRatePtr
	}
	return defaultRate, nil, defaultRatePtr
}

func cheapestSub2GroupsByPlatform(groups []sub2Group, userRates map[string]float64, platforms string) []sub2CheapestGroup {
	platformFilter := parseCSVFilter(platforms)
	cheapest := map[string]sub2CheapestGroup{}
	for _, group := range groups {
		if !sub2GroupMatches(group, nil, platformFilter) {
			continue
		}
		platform := strings.ToLower(strings.TrimSpace(group.Platform))
		if platform == "" {
			platform = "unknown"
		}
		rate, userRate, defaultRate := effectiveSub2GroupRate(group, userRates)
		candidate := sub2CheapestGroup{
			Platform:         platform,
			PlatformLabel:    sub2PlatformLabel(platform),
			GroupID:          group.ID,
			GroupName:        group.Name,
			GroupStatus:      group.Status,
			GroupDefaultRate: defaultRate,
			UserGroupRate:    userRate,
			EffectiveRate:    rate,
		}
		current, ok := cheapest[platform]
		if !ok || candidate.EffectiveRate < current.EffectiveRate ||
			(candidate.EffectiveRate == current.EffectiveRate && candidate.GroupName < current.GroupName) {
			cheapest[platform] = candidate
		}
	}
	platformsOut := make([]string, 0, len(cheapest))
	for platform := range cheapest {
		platformsOut = append(platformsOut, platform)
	}
	sort.Strings(platformsOut)
	out := make([]sub2CheapestGroup, 0, len(platformsOut))
	for _, platform := range platformsOut {
		out = append(out, cheapest[platform])
	}
	return out
}

func sub2PlatformLabel(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "anthropic":
		return "Claude"
	case "openai":
		return "GPT/OpenAI"
	case "gemini":
		return "Gemini"
	case "antigravity":
		return "Antigravity"
	case "":
		return "未知平台"
	default:
		return platform
	}
}

func officialPrice(entry map[string]any, key string) *float64 {
	value, ok := nullableFloat(entry[key])
	if !ok || value == 0 {
		return nil
	}
	return ptr(value)
}

func nullableFloat(value any) (float64, bool) {
	if value == nil {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		value, err := typed.Float64()
		return value, err == nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return 0, false
		}
		value, err := strconv.ParseFloat(typed, 64)
		return value, err == nil
	default:
		return 0, false
	}
}

func perMillionPtr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	return ptr(*value * pricePerMillion)
}

func multiplyPerMillionPtr(value *float64, rate float64) *float64 {
	if value == nil {
		return nil
	}
	return ptr(*value * rate * pricePerMillion)
}

func nullablePriceValue(values ...*float64) float64 {
	for _, value := range values {
		if value != nil {
			return *value
		}
	}
	return 1e308
}
