package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func priceComparisonExpr(hitRatioPlaceholder string) string {
	return "(CASE WHEN cache_write_price IS NULL AND cache_read_price IS NULL AND input_price IS NULL " +
		"THEN COALESCE(request_price, output_price, 1e308) " +
		"ELSE COALESCE(cache_write_price, input_price, request_price, output_price, 1e308) * (1 - " + hitRatioPlaceholder + ") + " +
		"COALESCE(cache_read_price, cache_write_price, input_price, request_price, output_price, 1e308) * " + hitRatioPlaceholder + " + " +
		"COALESCE(output_price, 0) END)"
}

func ApplyNewAPIUserGroupPricing(pricing map[string]any, groups map[string]NewAPIUserGroupPricing) {
	if pricing == nil || len(groups) == 0 {
		return
	}
	groupRatio := asMap(pricing["group_ratio"])
	if len(groupRatio) == 0 {
		groupRatio = map[string]any{}
		pricing["group_ratio"] = groupRatio
	}
	usableGroup := asMap(pricing["usable_group"])
	if len(usableGroup) == 0 {
		usableGroup = map[string]any{}
		pricing["usable_group"] = usableGroup
	}

	for name, group := range groups {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if group.Ratio != nil {
			setGroupMapValue(groupRatio, name, *group.Ratio)
		}
		if strings.TrimSpace(group.Desc) != "" {
			setGroupMapValue(usableGroup, name, strings.TrimSpace(group.Desc))
		}
	}
}

func BuildPricingRows(pricing map[string]any, wantedModel, wantedGroup string) ([]PricingRow, error) {
	groupRatio := asMap(pricing["group_ratio"])
	usableGroup := asMap(pricing["usable_group"])
	models, ok := pricing["data"].([]any)
	if !ok {
		return nil, fmt.Errorf("价格接口响应缺少 data 数组")
	}

	var rows []PricingRow
	for _, item := range models {
		model := asMap(item)
		modelName := stringValue(model["model_name"])
		if modelName == "" || modelName != wantedModel {
			continue
		}

		quotaType := int(floatValue(model["quota_type"], 0))
		modelRatio := floatValue(model["model_ratio"], 0)
		completionRatio := floatValue(model["completion_ratio"], 0)
		modelPrice := floatValue(model["model_price"], 0)

		for _, group := range enabledGroups(model, groupRatio) {
			if group != wantedGroup {
				continue
			}
			ratio := floatValue(groupRatio[group], 1)
			desc := groupDescription(usableGroup[group])
			row := PricingRow{
				ModelName:  modelName,
				GroupName:  group,
				GroupDesc:  desc,
				QuotaType:  quotaType,
				GroupRatio: ratio,
			}

			if quotaType == 1 {
				row.RequestPrice = ptr(modelPrice * ratio)
			} else {
				input := modelRatio * 2 * ratio
				output := input * completionRatio
				row.InputPrice = ptr(input)
				row.OutputPrice = ptr(output)
				if hasFloat(model["cache_ratio"]) {
					row.CacheReadPrice = ptr(input * floatValue(model["cache_ratio"], 0))
				}
				if hasFloat(model["create_cache_ratio"]) {
					row.CacheWritePrice = ptr(input * floatValue(model["create_cache_ratio"], 0))
				}
			}
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func BuildCheapestKeywordRows(pricing map[string]any, wantedModel string) ([]PricingRow, error) {
	rows, err := BuildKeywordRows(pricing, wantedModel)
	if err != nil {
		return nil, err
	}
	return CheapestPricingRows(rows), nil
}

func BuildCheapestKeywordRowsWithExpectedCacheHitRatio(pricing map[string]any, wantedModel string, expectedCacheHitRatio float64) ([]PricingRow, error) {
	rows, err := BuildKeywordRowsWithExpectedCacheHitRatio(pricing, wantedModel, expectedCacheHitRatio)
	if err != nil {
		return nil, err
	}
	return CheapestPricingRowsWithExpectedCacheHitRatio(rows, expectedCacheHitRatio), nil
}

func BuildKeywordRows(pricing map[string]any, wantedModel string) ([]PricingRow, error) {
	rows, err := BuildKeywordRowsRaw(pricing, wantedModel)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if pricingRowLess(rows[i], rows[j]) {
			return true
		}
		if pricingRowLess(rows[j], rows[i]) {
			return false
		}
		return rows[i].ModelName < rows[j].ModelName
	})
	return rows, nil
}

func BuildKeywordRowsWithExpectedCacheHitRatio(pricing map[string]any, wantedModel string, expectedCacheHitRatio float64) ([]PricingRow, error) {
	rows, err := BuildKeywordRowsRaw(pricing, wantedModel)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if pricingRowLessWithExpectedCacheHitRatio(rows[i], rows[j], expectedCacheHitRatio) {
			return true
		}
		if pricingRowLessWithExpectedCacheHitRatio(rows[j], rows[i], expectedCacheHitRatio) {
			return false
		}
		return rows[i].ModelName < rows[j].ModelName
	})
	return rows, nil
}

func BuildKeywordRowsRaw(pricing map[string]any, wantedModel string) ([]PricingRow, error) {
	wantedModel = strings.TrimSpace(wantedModel)
	if wantedModel == "" {
		return nil, fmt.Errorf("模型名称不能为空")
	}
	groupRatio := asMap(pricing["group_ratio"])
	usableGroup := asMap(pricing["usable_group"])
	models, ok := pricing["data"].([]any)
	if !ok {
		return nil, fmt.Errorf("价格接口响应缺少 data 数组")
	}

	var rows []PricingRow
	for _, item := range models {
		model := asMap(item)
		modelName := stringValue(model["model_name"])
		if modelName == "" || !strings.EqualFold(strings.TrimSpace(modelName), wantedModel) {
			continue
		}

		for _, group := range enabledGroups(model, groupRatio) {
			row := buildPricingRow(model, usableGroup, groupRatio, modelName, group)
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func CheapestPricingRows(rows []PricingRow) []PricingRow {
	return CheapestPricingRowsWithExpectedCacheHitRatio(rows, 0)
}

func CheapestPricingRowsWithExpectedCacheHitRatio(rows []PricingRow, expectedCacheHitRatio float64) []PricingRow {
	cheapest := map[string]PricingRow{}
	for _, row := range rows {
		if strings.TrimSpace(row.ModelName) == "" {
			continue
		}
		current, ok := cheapest[row.ModelName]
		if !ok || pricingRowLessWithExpectedCacheHitRatio(row, current, expectedCacheHitRatio) {
			cheapest[row.ModelName] = row
		}
	}
	models := make([]string, 0, len(cheapest))
	for model := range cheapest {
		models = append(models, model)
	}
	sort.Strings(models)
	out := make([]PricingRow, 0, len(models))
	for _, model := range models {
		out = append(out, cheapest[model])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if pricingRowLessWithExpectedCacheHitRatio(out[i], out[j], expectedCacheHitRatio) {
			return true
		}
		if pricingRowLessWithExpectedCacheHitRatio(out[j], out[i], expectedCacheHitRatio) {
			return false
		}
		return out[i].ModelName < out[j].ModelName
	})
	return out
}

func buildPricingRow(model map[string]any, usableGroup map[string]any, groupRatio map[string]any, modelName string, group string) PricingRow {
	quotaType := int(floatValue(model["quota_type"], 0))
	modelRatio := floatValue(model["model_ratio"], 0)
	completionRatio := floatValue(model["completion_ratio"], 0)
	modelPrice := floatValue(model["model_price"], 0)
	ratio := floatValue(groupRatio[group], 1)
	row := PricingRow{
		ModelName:  modelName,
		GroupName:  group,
		GroupDesc:  groupDescription(usableGroup[group]),
		QuotaType:  quotaType,
		GroupRatio: ratio,
	}
	if quotaType == 1 {
		row.RequestPrice = ptr(modelPrice * ratio)
		return row
	}

	input := modelRatio * 2 * ratio
	output := input * completionRatio
	row.InputPrice = ptr(input)
	row.OutputPrice = ptr(output)
	if hasFloat(model["cache_ratio"]) {
		row.CacheReadPrice = ptr(input * floatValue(model["cache_ratio"], 0))
	}
	if hasFloat(model["create_cache_ratio"]) {
		row.CacheWritePrice = ptr(input * floatValue(model["create_cache_ratio"], 0))
	}
	return row
}

func pricingRowLess(left, right PricingRow) bool {
	leftPrice := effectivePrice(left)
	rightPrice := effectivePrice(right)
	if leftPrice != rightPrice {
		return leftPrice < rightPrice
	}
	if left.OutputPrice != nil && right.OutputPrice != nil && *left.OutputPrice != *right.OutputPrice {
		return *left.OutputPrice < *right.OutputPrice
	}
	if left.GroupRatio != right.GroupRatio {
		return left.GroupRatio < right.GroupRatio
	}
	return left.GroupName < right.GroupName
}

func effectivePrice(row PricingRow) float64 {
	if row.InputPrice != nil {
		return *row.InputPrice
	}
	if row.RequestPrice != nil {
		return *row.RequestPrice
	}
	if row.OutputPrice != nil {
		return *row.OutputPrice
	}
	return 1e308
}

func pricingRowLessWithExpectedCacheHitRatio(left, right PricingRow, expectedCacheHitRatio float64) bool {
	leftPrice := pricingRowExpectedPrice(left, expectedCacheHitRatio)
	rightPrice := pricingRowExpectedPrice(right, expectedCacheHitRatio)
	if leftPrice != rightPrice {
		return leftPrice < rightPrice
	}
	if left.OutputPrice != nil && right.OutputPrice != nil && *left.OutputPrice != *right.OutputPrice {
		return *left.OutputPrice < *right.OutputPrice
	}
	if left.GroupRatio != right.GroupRatio {
		return left.GroupRatio < right.GroupRatio
	}
	return left.GroupName < right.GroupName
}

func pricingRowExpectedPrice(row PricingRow, expectedCacheHitRatio float64) float64 {
	hitRatio := normalizeExpectedCacheHitRatio(expectedCacheHitRatio)
	if row.InputPrice == nil && row.CacheReadPrice == nil && row.CacheWritePrice == nil {
		if row.RequestPrice != nil {
			return *row.RequestPrice
		}
		if row.OutputPrice != nil {
			return *row.OutputPrice
		}
		return 1e308
	}
	missPrice := firstComparablePrice(row.CacheWritePrice, row.InputPrice, row.RequestPrice, row.OutputPrice)
	hitPrice := firstComparablePrice(row.CacheReadPrice, row.CacheWritePrice, row.InputPrice, row.RequestPrice, row.OutputPrice)
	if missPrice == 1e308 && hitPrice == 1e308 {
		return 1e308
	}
	if missPrice == 1e308 {
		missPrice = hitPrice
	}
	if hitPrice == 1e308 {
		hitPrice = missPrice
	}
	expected := missPrice*(1-hitRatio) + hitPrice*hitRatio
	if row.InputPrice != nil && row.OutputPrice != nil {
		expected += *row.OutputPrice
	}
	return expected
}

func firstComparablePrice(values ...*float64) float64 {
	for _, value := range values {
		if value != nil {
			return *value
		}
	}
	return 1e308
}

func PricingRowRaw(row PricingRow) []byte {
	data, err := json.Marshal(row)
	if err != nil {
		return []byte(`{}`)
	}
	return data
}

func enabledGroups(model map[string]any, groupRatio map[string]any) []string {
	rawGroups, ok := model["enable_groups"].([]any)
	if !ok {
		rawGroups, _ = model["enable_group"].([]any)
	}
	if len(rawGroups) == 0 {
		groups := make([]string, 0, len(groupRatio))
		for group := range groupRatio {
			groups = append(groups, group)
		}
		sort.Strings(groups)
		return groups
	}

	for _, raw := range rawGroups {
		if stringValue(raw) == "all" {
			groups := make([]string, 0, len(groupRatio))
			for group := range groupRatio {
				groups = append(groups, group)
			}
			sort.Strings(groups)
			return groups
		}
	}

	var groups []string
	for _, raw := range rawGroups {
		group := stringValue(raw)
		if _, ok := groupRatio[group]; ok {
			groups = append(groups, group)
		}
	}
	return groups
}

func asMap(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func setGroupMapValue(values map[string]any, name string, value any) {
	if _, ok := values[name]; ok {
		values[name] = value
		return
	}
	normalized := strings.ToLower(name)
	for key := range values {
		if strings.ToLower(strings.TrimSpace(key)) == normalized {
			values[key] = value
			return
		}
	}
	values[name] = value
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func groupDescription(value any) string {
	group := asMap(value)
	if desc := stringValue(group["desc"]); desc != "" {
		return desc
	}
	return stringValue(value)
}

func floatValue(value any, fallback float64) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		value, err := typed.Float64()
		if err == nil {
			return value
		}
	case string:
		value, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return value
		}
	}
	return fallback
}

func hasFloat(value any) bool {
	if value == nil {
		return false
	}
	switch value.(type) {
	case float64, float32, int, int64, json.Number:
		return true
	case string:
		_, err := strconv.ParseFloat(strings.TrimSpace(value.(string)), 64)
		return err == nil
	default:
		return false
	}
}

func ptr(value float64) *float64 {
	return &value
}
