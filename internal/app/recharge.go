package app

import (
	"math"
	"sort"
	"strings"
)

func addRechargeMultiplier(best **float64, credited, paid float64) {
	if credited <= 0 || paid <= 0 || math.IsNaN(credited) || math.IsNaN(paid) || math.IsInf(credited, 0) || math.IsInf(paid, 0) {
		return
	}
	ratio := credited / paid
	if ratio <= 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return
	}
	if *best == nil || **best <= 0 || ratio > **best {
		value := ratio
		*best = &value
	}
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on", "enabled":
			return true
		default:
			return false
		}
	case float64:
		return typed != 0
	case int:
		return typed != 0
	default:
		return false
	}
}

func positiveAmountsFromTopupInfo(info map[string]any, keys ...string) []int64 {
	seen := map[int64]bool{}
	for _, key := range keys {
		if value, ok := nullableFloat(info[key]); ok && value > 0 {
			seen[int64(math.Ceil(value))] = true
		}
	}
	if raw, ok := info["amount_options"].([]any); ok {
		for _, item := range raw {
			if value, ok := nullableFloat(item); ok && value > 0 {
				seen[int64(math.Ceil(value))] = true
			}
		}
	}
	if len(seen) == 0 {
		seen[1] = true
		seen[10] = true
		seen[100] = true
	}
	amounts := make([]int64, 0, len(seen))
	for amount := range seen {
		if amount > 0 {
			amounts = append(amounts, amount)
		}
	}
	sort.Slice(amounts, func(i, j int) bool { return amounts[i] < amounts[j] })
	if len(amounts) > 8 {
		amounts = amounts[:8]
	}
	return amounts
}
