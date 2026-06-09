package app

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

type storedCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HttpOnly bool   `json:"http_only,omitempty"`
}

func cookieURL(baseURL string) (*url.URL, error) {
	parsed, err := url.Parse(normalizeBaseURL(baseURL))
	if err != nil {
		return nil, err
	}
	parsed.Path = "/"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func exportCookies(baseURL string, client *http.Client) string {
	if client == nil || client.Jar == nil {
		return ""
	}
	parsed, err := cookieURL(baseURL)
	if err != nil {
		return ""
	}
	cookies := client.Jar.Cookies(parsed)
	if len(cookies) == 0 {
		return ""
	}
	out := make([]storedCookie, 0, len(cookies))
	for _, cookie := range cookies {
		if strings.TrimSpace(cookie.Name) == "" || cookie.Value == "" {
			continue
		}
		out = append(out, storedCookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Path:     cookie.Path,
			Domain:   cookie.Domain,
			Secure:   cookie.Secure,
			HttpOnly: cookie.HttpOnly,
		})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(raw)
}

func importCookies(baseURL string, client *http.Client, raw string) error {
	if client == nil || client.Jar == nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var stored []storedCookie
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return err
	}
	parsed, err := cookieURL(baseURL)
	if err != nil {
		return err
	}
	cookies := make([]*http.Cookie, 0, len(stored))
	for _, item := range stored {
		if strings.TrimSpace(item.Name) == "" || item.Value == "" {
			continue
		}
		cookies = append(cookies, &http.Cookie{
			Name:     item.Name,
			Value:    item.Value,
			Path:     firstNonEmpty(item.Path, "/"),
			Domain:   item.Domain,
			Secure:   item.Secure,
			HttpOnly: item.HttpOnly,
		})
	}
	if len(cookies) > 0 {
		client.Jar.SetCookies(parsed, cookies)
	}
	return nil
}

func (c *NewAPIClient) LoadCookies(raw string) error {
	return importCookies(c.baseURL, c.client, raw)
}

func (c *NewAPIClient) DumpCookies() string {
	return exportCookies(c.baseURL, c.client)
}

func (c *Sub2APIClient) LoadCookies(raw string) error {
	return importCookies(c.baseURL, c.client, raw)
}

func (c *Sub2APIClient) DumpCookies() string {
	return exportCookies(c.baseURL, c.client)
}
