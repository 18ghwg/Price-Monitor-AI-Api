package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const modelLatencyProbeTimeout = 30 * time.Second

func probeModelRequestLatency(ctx context.Context, baseURL string, apiKey string, modelName string) (float64, error) {
	baseURL = normalizeBaseURL(baseURL)
	apiKey = strings.TrimSpace(apiKey)
	modelName = strings.TrimSpace(modelName)
	if baseURL == "" || apiKey == "" || modelName == "" {
		return 0, fmt.Errorf("base url, api key and model are required for latency probe")
	}

	probeCtx, cancel := context.WithTimeout(ctx, modelLatencyProbeTimeout)
	defer cancel()
	payload := map[string]any{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": "hi"},
		},
		"stream":     false,
		"max_tokens": 1,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	endpoint, err := url.JoinPath(baseURL, "v1/chat/completions")
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(probeCtx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", newAPIUserAgent)
	req.Header.Set("Authorization", "Bearer "+trimBearerPrefix(apiKey))

	client := &http.Client{Timeout: modelLatencyProbeTimeout}
	start := time.Now()
	resp, err := client.Do(req)
	elapsed := float64(time.Since(start).Microseconds()) / 1000.0
	if err != nil {
		return 0, fmt.Errorf("request %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, localizedHTTPError("上游模型延迟测试", endpoint, resp.StatusCode, body)
	}
	if !json.Valid(body) {
		return 0, fmt.Errorf("upstream latency probe returned non-json response")
	}
	return elapsed, nil
}

func effectiveLatencyWeight(settings IntegrationSettings) float64 {
	if !settings.LatencyTestEnabled {
		return 0
	}
	weight := normalizeLatencyWeightPerSecond(settings.LatencyWeightPerSecond)
	if weight <= 0 {
		return 0.1
	}
	return weight
}
