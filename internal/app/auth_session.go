package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

func isSessionAuthError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	needles := []string{
		"http 401",
		"http 403",
		"unauthorized",
		"forbidden",
		"invalid token",
		"invalid_token",
		"token expired",
		"jwt",
		"not logged in",
		"login required",
		"未登录",
		"登录已过期",
		"登录过期",
		"认证失败",
		"未授权",
		"无权限",
	}
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func (s *Server) newAPIClientForSite(ctx context.Context, site Site, forceLogin bool) (*NewAPIClient, int64, string, error) {
	client, err := NewNewAPIClient(site.BaseURL)
	if err != nil {
		return nil, 0, "", err
	}
	if err := client.LoadCookies(site.CookieJar); err != nil {
		log.Printf("load newapi cookies for site %d: %v", site.ID, err)
	}
	userID := site.UserID
	token := strings.TrimSpace(site.AccessToken)
	if !forceLogin {
		if userID > 0 && token != "" {
			return client, userID, token, nil
		}
		if userID > 0 {
			generated, genErr := client.GenerateSystemAccessToken(ctx, userID)
			if genErr == nil {
				return client, userID, generated, nil
			}
			if !isSessionAuthError(genErr) {
				log.Printf("generate newapi token from stored cookie for site %d: %v", site.ID, genErr)
			}
		}
	}
	if strings.TrimSpace(site.Password) == "" {
		if token != "" {
			return client, userID, token, fmt.Errorf("NewAPI 系统访问令牌已失效，请在站点编辑中更新令牌")
		}
		return client, userID, token, fmt.Errorf("NewAPI 上游未保存密码，无法重新登录，请填写系统访问令牌或密码")
	}
	userID, err = client.Login(ctx, site.Username, site.Password, site.TOTPCode)
	if err != nil {
		return client, site.UserID, token, fmt.Errorf("login NewAPI upstream: %w", err)
	}
	token, err = client.GenerateSystemAccessToken(ctx, userID)
	if err != nil {
		return client, userID, "", fmt.Errorf("generate NewAPI system token: %w", err)
	}
	return client, userID, token, nil
}

func (s *Server) saveNewAPISession(ctx context.Context, site Site, client *NewAPIClient, userID int64, token string, lastErr string) {
	if site.ID <= 0 {
		return
	}
	cookieJar := ""
	if client != nil {
		cookieJar = client.DumpCookies()
	}
	if err := s.store.UpdateSiteRunWithCookies(ctx, site.ID, userID, token, cookieJar, time.Now(), lastErr); err != nil {
		log.Printf("save newapi session for site %d: %v", site.ID, err)
	}
}

type sub2APIUserSourceConfig struct {
	UpstreamID     int64
	BaseURL        string
	AuthToken      string
	Email          string
	Password       string
	TOTPCode       string
	TurnstileToken string
	CookieJar      string
}

func (s *Server) sub2APIUserSourceConfig(ctx context.Context, input sub2APIUserPriceInput) (sub2APIUserSourceConfig, error) {
	if input.Sub2APIUpstreamID > 0 {
		upstream, err := s.store.GetSub2APIUpstream(ctx, input.Sub2APIUpstreamID)
		if err != nil {
			return sub2APIUserSourceConfig{}, fmt.Errorf("load sub2api upstream: %w", err)
		}
		return sub2APIUserSourceConfig{
			UpstreamID:     upstream.ID,
			BaseURL:        upstream.BaseURL,
			AuthToken:      firstNonEmpty(input.AuthToken, upstream.AuthToken),
			Email:          firstNonEmpty(input.Email, upstream.Email),
			Password:       firstNonEmpty(input.Password, upstream.Password),
			TOTPCode:       firstNonEmpty(input.TOTPCode, upstream.TOTPCode),
			TurnstileToken: input.TurnstileToken,
			CookieJar:      upstream.CookieJar,
		}, nil
	}
	baseURL := normalizeBaseURL(input.BaseURL)
	if baseURL == "" {
		settings, err := s.store.GetIntegrationSettings(ctx)
		if err != nil {
			return sub2APIUserSourceConfig{}, fmt.Errorf("load integration settings: %w", err)
		}
		baseURL = firstNonEmpty(settings.Sub2APIMainBaseURL, settings.Sub2APIBaseURL)
	}
	return sub2APIUserSourceConfig{
		BaseURL:        baseURL,
		AuthToken:      input.AuthToken,
		Email:          input.Email,
		Password:       input.Password,
		TOTPCode:       input.TOTPCode,
		TurnstileToken: input.TurnstileToken,
	}, nil
}

func (s *Server) sub2APIClientForUserSource(ctx context.Context, cfg sub2APIUserSourceConfig, forceLogin bool) (*Sub2APIClient, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("sub2api upstream base url is not configured")
	}
	client, err := NewSub2APIClient(cfg.BaseURL, cfg.AuthToken)
	if err != nil {
		return nil, err
	}
	if err := client.LoadCookies(cfg.CookieJar); err != nil {
		log.Printf("load sub2api cookies for upstream %d: %v", cfg.UpstreamID, err)
	}
	if !forceLogin && (strings.TrimSpace(cfg.AuthToken) != "" || strings.TrimSpace(cfg.CookieJar) != "") {
		return client, nil
	}
	if err := client.LoginWith2FA(ctx, cfg.Email, cfg.Password, cfg.TOTPCode, cfg.TurnstileToken); err != nil {
		return client, err
	}
	return client, nil
}

func (s *Server) saveSub2APIUserSession(ctx context.Context, cfg sub2APIUserSourceConfig, client *Sub2APIClient, lastErr string) {
	if cfg.UpstreamID <= 0 {
		return
	}
	cookieJar := ""
	authToken := ""
	if client != nil {
		cookieJar = client.DumpCookies()
	}
	if err := s.store.UpdateSub2APIUpstreamCheckWithSession(ctx, cfg.UpstreamID, time.Now(), lastErr, cookieJar, authToken); err != nil {
		log.Printf("save sub2api session for upstream %d: %v", cfg.UpstreamID, err)
	}
}
