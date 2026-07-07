package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Server struct {
	cfg         Config
	db          *pgxpool.Pool
	store       Store
	secret      []byte
	pollMu      sync.Mutex
	pollRunning bool
}

const (
	syncFailurePauseThreshold = 3
	syncCandidateTimeout      = 75 * time.Second
)

func NewServer(ctx context.Context, cfg Config) (*Server, error) {
	db, err := openDB(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	server := &Server{
		cfg:    cfg,
		db:     db,
		store:  Store{db: db},
		secret: []byte(firstNonEmpty(cfg.SessionSecret, cfg.BasicAuthPass, "newapi-price-monitor-dev-secret")),
	}
	if err := server.ensureDefaultAdmin(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if cfg.MonitorInterval > 0 {
		log.Printf("price monitor scheduler enabled, fallback interval %s", cfg.MonitorInterval)
		go server.startScheduler(context.Background(), cfg.MonitorInterval)
	}
	return server, nil
}

func (s *Server) Close() {
	s.db.Close()
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /", s.index)
	mux.HandleFunc("GET /static/app.css", s.styles)
	mux.HandleFunc("GET /static/app.js", s.script)
	mux.HandleFunc("GET /api/auth/session", s.authSession)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("POST /api/auth/password", s.updateAdminPassword)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.saveSettings)
	mux.HandleFunc("POST /api/settings", s.saveSettings)
	mux.HandleFunc("GET /api/sub2api/groups", s.listSub2APIGroups)
	mux.HandleFunc("GET /api/sub2api/accounts", s.listSub2APIAccounts)
	mux.HandleFunc("GET /api/sub2api/upstreams", s.listSub2APIUpstreams)
	mux.HandleFunc("POST /api/sub2api/upstreams", s.createSub2APIUpstream)
	mux.HandleFunc("PUT /api/sub2api/upstreams/{id}", s.updateSub2APIUpstream)
	mux.HandleFunc("DELETE /api/sub2api/upstreams/{id}", s.deleteSub2APIUpstream)
	mux.HandleFunc("POST /api/sub2api/upstreams/{id}/update", s.updateSub2APIUpstream)
	mux.HandleFunc("POST /api/sub2api/upstreams/{id}/delete", s.deleteSub2APIUpstream)
	mux.HandleFunc("POST /api/sub2api/user/inspect", s.inspectSub2APIUserHandler)
	mux.HandleFunc("POST /api/sub2api/user-filter-options", s.fetchSub2APIUserFilterOptionsHandler)
	mux.HandleFunc("POST /api/sub2api/user-prices", s.fetchSub2APIUserPricesHandler)
	mux.HandleFunc("POST /api/model-probe", s.fetchModelProbeHandler)
	mux.HandleFunc("POST /api/sub2api/accounts/upsert", s.upsertSub2APIAccount)
	mux.HandleFunc("POST /api/sub2api/accounts/{id}/enable", s.enableSub2APIAccount)
	mux.HandleFunc("POST /api/sub2api/accounts/{id}/disable", s.disableSub2APIAccount)
	mux.HandleFunc("POST /api/sub2api/accounts/{id}/apikey", s.updateSub2APIAccountKey)
	mux.HandleFunc("GET /api/sites", s.listSites)
	mux.HandleFunc("POST /api/sites/refresh-tokens", s.refreshNewAPISiteTokens)
	mux.HandleFunc("POST /api/sites", s.createSite)
	mux.HandleFunc("PUT /api/sites/{id}", s.updateSite)
	mux.HandleFunc("DELETE /api/sites/{id}", s.deleteSite)
	mux.HandleFunc("POST /api/sites/{id}/update", s.updateSite)
	mux.HandleFunc("POST /api/sites/{id}/delete", s.deleteSite)
	mux.HandleFunc("GET /api/categories", s.listCategories)
	mux.HandleFunc("POST /api/categories", s.createCategory)
	mux.HandleFunc("PUT /api/categories/{id}", s.updateCategory)
	mux.HandleFunc("DELETE /api/categories/{id}", s.deleteCategory)
	mux.HandleFunc("POST /api/categories/{id}/update", s.updateCategory)
	mux.HandleFunc("POST /api/categories/{id}/delete", s.deleteCategory)
	mux.HandleFunc("GET /api/rules", s.listRules)
	mux.HandleFunc("POST /api/rules", s.createRule)
	mux.HandleFunc("POST /api/rules/bulk-create", s.bulkCreateRules)
	mux.HandleFunc("POST /api/rules/bulk-create-claude", s.bulkCreateClaudeRules)
	mux.HandleFunc("POST /api/rules/bulk-update", s.bulkUpdateRules)
	mux.HandleFunc("POST /api/rules/run-poll", s.runRulePoll)
	mux.HandleFunc("PUT /api/rules/{id}", s.updateRule)
	mux.HandleFunc("DELETE /api/rules/{id}", s.deleteRule)
	mux.HandleFunc("POST /api/rules/{id}/update", s.updateRule)
	mux.HandleFunc("POST /api/rules/{id}/delete", s.deleteRule)
	mux.HandleFunc("POST /api/rules/{id}/run", s.runRule)
	mux.HandleFunc("GET /api/snapshots", s.listSnapshots)
	return s.auth(mux)
}

func (s *Server) auth(next http.Handler) http.Handler {
	if !s.authConfigured() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/static/") ||
			r.URL.Path == "/api/auth/login" || r.URL.Path == "/api/auth/logout" || r.URL.Path == "/api/auth/session" {
			next.ServeHTTP(w, r)
			return
		}
		if s.validSession(r) {
			next.ServeHTTP(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || !s.validateAdminPassword(r.Context(), user, pass) {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.validateAdminPassword(r.Context(), input.Username, input.Password) {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "pm_session",
		Value:    s.signSession(time.Now().Add(12 * time.Hour)),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(12 * time.Hour),
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"authenticated": true}})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "pm_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"authenticated": false}})
}

func (s *Server) authSession(w http.ResponseWriter, r *http.Request) {
	authenticated := s.validSession(r)
	if !authenticated {
		user, pass, ok := r.BasicAuth()
		authenticated = ok && s.validateAdminPassword(r.Context(), user, pass)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"authenticated": authenticated}})
}

func (s *Server) updateAdminPassword(w http.ResponseWriter, r *http.Request) {
	var input AdminCredentialInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	username := strings.TrimSpace(input.Username)
	currentPassword := strings.TrimSpace(input.CurrentPassword)
	newPassword := strings.TrimSpace(input.NewPassword)
	if username == "" || currentPassword == "" || newPassword == "" {
		writeError(w, http.StatusBadRequest, "username, current password and new password are required")
		return
	}
	if len(newPassword) < 6 {
		writeError(w, http.StatusBadRequest, "new password must be at least 6 characters")
		return
	}
	if !s.validateAdminPassword(r.Context(), s.currentAdminUsername(r.Context()), currentPassword) {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	passwordHash, err := hashPassword(newPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash password failed")
		return
	}
	credentials, err := s.store.SaveAdminCredentials(r.Context(), username, passwordHash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save admin credentials failed")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "pm_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"username":      credentials.Username,
		"authenticated": false,
	}})
}

func (s *Server) ensureDefaultAdmin(ctx context.Context) error {
	if strings.TrimSpace(s.cfg.BasicAuthUser) == "" || strings.TrimSpace(s.cfg.BasicAuthPass) == "" {
		return nil
	}
	passwordHash, err := hashPassword(s.cfg.BasicAuthPass)
	if err != nil {
		return fmt.Errorf("hash default admin password: %w", err)
	}
	if err := s.store.EnsureAdminCredentials(ctx, s.cfg.BasicAuthUser, passwordHash); err != nil {
		return fmt.Errorf("seed admin credentials: %w", err)
	}
	return nil
}

func (s *Server) authConfigured() bool {
	if strings.TrimSpace(s.cfg.BasicAuthUser) != "" || strings.TrimSpace(s.cfg.BasicAuthPass) != "" {
		return true
	}
	credentials, err := s.store.GetAdminCredentials(context.Background())
	return err == nil && strings.TrimSpace(credentials.Username) != "" && strings.TrimSpace(credentials.PasswordHash) != ""
}

func (s *Server) validateAdminPassword(ctx context.Context, username string, password string) bool {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return false
	}
	credentials, err := s.store.GetAdminCredentials(ctx)
	if err == nil && credentials.Username != "" && credentials.PasswordHash != "" {
		userOK := subtle.ConstantTimeCompare([]byte(username), []byte(credentials.Username)) == 1
		passOK := bcrypt.CompareHashAndPassword([]byte(credentials.PasswordHash), []byte(password)) == nil
		return userOK && passOK
	}
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(s.cfg.BasicAuthUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(password), []byte(s.cfg.BasicAuthPass)) == 1
	return userOK && passOK
}

func (s *Server) currentAdminUsername(ctx context.Context) string {
	credentials, err := s.store.GetAdminCredentials(ctx)
	if err == nil && strings.TrimSpace(credentials.Username) != "" {
		return credentials.Username
	}
	return strings.TrimSpace(s.cfg.BasicAuthUser)
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (s *Server) signSession(expires time.Time) string {
	nonce := make([]byte, 18)
	if _, err := rand.Read(nonce); err != nil {
		nonce = []byte(strconv.FormatInt(time.Now().UnixNano(), 10))
	}
	payload := fmt.Sprintf("%d.%d.%s", expires.Unix(), time.Now().UnixNano(), base64.RawURLEncoding.EncodeToString(nonce))
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload + "." + sig))
}

func (s *Server) validSession(r *http.Request) bool {
	cookie, err := r.Cookie("pm_session")
	if err != nil || cookie.Value == "" {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return false
	}
	parts := strings.Split(string(raw), ".")
	if len(parts) != 4 {
		return false
	}
	expires, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return false
	}
	issued, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return false
	}
	credentials, err := s.store.GetAdminCredentials(r.Context())
	if err == nil && credentials.UpdatedAt.UnixNano() > issued {
		return false
	}
	payload := parts[0] + "." + parts[1] + "." + parts[2]
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(payload))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(parts[3]))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "postgres unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := indexTemplate.Execute(w, nil); err != nil {
		log.Printf("render index: %v", err)
	}
}

func (s *Server) styles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(appCSS))
}

func (s *Server) script(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(appJS))
}

func (s *Server) listSites(w http.ResponseWriter, r *http.Request) {
	sites, err := s.store.ListSites(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list sites failed")
		return
	}
	upstreams, err := s.store.ListSub2APIUpstreams(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list sub2api upstreams failed")
		return
	}
	type siteDTO struct {
		ID             int64      `json:"id"`
		SourceType     string     `json:"source_type"`
		Name           string     `json:"name"`
		BaseURL        string     `json:"base_url"`
		Username       string     `json:"username"`
		UserID         int64      `json:"user_id,omitempty"`
		Email          string     `json:"email,omitempty"`
		HasAccessToken bool       `json:"has_access_token,omitempty"`
		LastError      string     `json:"last_error"`
		LastRunAt      *time.Time `json:"last_run_at"`
	}
	out := make([]siteDTO, 0, len(sites)+len(upstreams))
	for _, site := range sites {
		out = append(out, siteDTO{
			ID: site.ID, SourceType: RuleSourceNewAPI, Name: site.Name, BaseURL: site.BaseURL, Username: site.Username, UserID: site.UserID,
			HasAccessToken: strings.TrimSpace(site.AccessToken) != "", LastError: site.LastError, LastRunAt: site.LastRunAt,
		})
	}
	for _, upstream := range upstreams {
		out = append(out, siteDTO{
			ID: upstream.ID, SourceType: RuleSourceSub2API, Name: upstream.Name, BaseURL: upstream.BaseURL,
			Username: upstream.Email, Email: upstream.Email, LastError: upstream.LastError, LastRunAt: upstream.LastCheckAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetIntegrationSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load settings failed")
		return
	}
	settings.Sub2APIPassword = ""
	settings.Sub2APIAdminKey = maskSecret(settings.Sub2APIAdminKey)
	settings.Sub2APIAccessToken = maskSecret(settings.Sub2APIAccessToken)
	settings.SMTPPassword = maskSecret(settings.SMTPPassword)
	writeJSON(w, http.StatusOK, map[string]any{"data": settings})
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	var input SettingsInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	settings, err := s.store.SaveIntegrationSettings(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if settings.Sub2APIEnabled {
		if err := s.store.RestoreRuleSyncDisabledByGlobalSwitch(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "restore rule sync failed")
			return
		}
	}
	settings.Sub2APIPassword = ""
	settings.Sub2APIAdminKey = maskSecret(settings.Sub2APIAdminKey)
	settings.Sub2APIAccessToken = maskSecret(settings.Sub2APIAccessToken)
	settings.SMTPPassword = maskSecret(settings.SMTPPassword)
	writeJSON(w, http.StatusOK, map[string]any{"data": settings})
}

func (s *Server) listSub2APIGroups(w http.ResponseWriter, r *http.Request) {
	client, err := s.sub2APIClient(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	groups, err := client.listGroups(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": groups})
}

func (s *Server) listSub2APIAccounts(w http.ResponseWriter, r *http.Request) {
	client, err := s.sub2APIClient(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	group, err := s.sub2GroupFromRequest(r.Context(), client, r)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	accounts, err := client.ListOpenAIAPIKeyAccounts(r.Context(), r.URL.Query().Get("apiurl"), group)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": redactSub2Accounts(accounts)})
}

func (s *Server) listSub2APIUpstreams(w http.ResponseWriter, r *http.Request) {
	upstreams, err := s.store.ListSub2APIUpstreams(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list sub2api upstreams failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": sub2APIUpstreamViews(upstreams)})
}

func (s *Server) createSub2APIUpstream(w http.ResponseWriter, r *http.Request) {
	var input Sub2APIUpstreamInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.ensureUniqueSub2APIUpstream(r.Context(), normalizeSub2APIUpstreamInput(input), 0); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	verified, inspect, err := s.verifySub2APIUpstreamInput(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	saved, err := s.store.CreateSub2APIUpstream(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	_ = s.store.UpdateSub2APIUpstreamCheckWithSession(r.Context(), saved.ID, time.Now(), "", verified.CookieJar, verified.AuthToken)
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"upstream": redactSub2APIUpstream(saved),
		"inspect":  inspect,
	}})
}

func (s *Server) updateSub2APIUpstream(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid sub2api upstream id")
		return
	}
	var input Sub2APIUpstreamInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	existing, err := s.store.GetSub2APIUpstream(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err := s.store.ensureUniqueSub2APIUpstream(r.Context(), normalizeSub2APIUpstreamInput(input), id); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	verifyInput := input
	if strings.TrimSpace(verifyInput.Password) == "" {
		verifyInput.Password = existing.Password
	}
	if strings.TrimSpace(verifyInput.AuthToken) == "" {
		verifyInput.AuthToken = existing.AuthToken
	}
	verified, inspect, err := s.verifySub2APIUpstreamInput(r.Context(), verifyInput)
	if err != nil {
		_ = s.store.UpdateSub2APIUpstreamCheck(r.Context(), id, time.Now(), err.Error())
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	upstream, err := s.store.UpdateSub2APIUpstream(r.Context(), id, input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	_ = s.store.UpdateSub2APIUpstreamCheckWithSession(r.Context(), id, time.Now(), "", verified.CookieJar, verified.AuthToken)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"upstream": redactSub2APIUpstream(upstream),
		"inspect":  inspect,
	}})
}

func (s *Server) deleteSub2APIUpstream(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid sub2api upstream id")
		return
	}
	if err := s.store.DeleteSub2APIUpstream(r.Context(), id); err != nil {
		status := http.StatusConflict
		if notFound(err) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) verifySub2APIUpstreamInput(ctx context.Context, input Sub2APIUpstreamInput) (Sub2APIUpstream, sub2APIUserInspectResult, error) {
	input = normalizeSub2APIUpstreamInput(input)
	upstream := Sub2APIUpstream{
		Name:      input.Name,
		BaseURL:   input.BaseURL,
		Email:     input.Email,
		Password:  input.Password,
		AuthToken: input.AuthToken,
		TOTPCode:  input.TOTPCode,
	}
	if upstream.Name == "" || upstream.BaseURL == "" {
		return upstream, sub2APIUserInspectResult{}, fmt.Errorf("upstream name and base url are required")
	}
	if upstream.AuthToken == "" && (upstream.Email == "" || upstream.Password == "") {
		return upstream, sub2APIUserInspectResult{}, fmt.Errorf("upstream username/password or auth token is required")
	}
	cfg := sub2APIUserSourceConfig{
		BaseURL:   upstream.BaseURL,
		AuthToken: upstream.AuthToken,
		Email:     upstream.Email,
		Password:  upstream.Password,
		TOTPCode:  upstream.TOTPCode,
	}
	client, groups, userRates, err := s.fetchSub2APIUserClientGroupsForSource(ctx, cfg)
	if err == nil && client != nil {
		upstream.CookieJar = client.DumpCookies()
	}
	inspect := sub2APIUserInspectResult{
		Groups:         groups,
		UserGroupRates: userRates,
		CheapestGroups: cheapestSub2GroupsByPlatform(groups, userRates, ""),
		Filters: map[string]string{
			"platforms": "",
		},
	}
	return upstream, inspect, err
}

func (s *Server) fetchSub2APIUserPricesHandler(w http.ResponseWriter, r *http.Request) {
	var input sub2APIUserPriceInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.Sub2APIUpstreamID <= 0 {
		if strings.TrimSpace(input.BaseURL) == "" {
			writeError(w, http.StatusBadRequest, "sub2api upstream base url is required")
			return
		}
		if strings.TrimSpace(input.AuthToken) == "" && (strings.TrimSpace(input.Email) == "" || strings.TrimSpace(input.Password) == "") {
			writeError(w, http.StatusBadRequest, "sub2api user username/password or auth token is required")
			return
		}
	}
	result, err := s.fetchSub2APIUserPrices(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (s *Server) inspectSub2APIUserHandler(w http.ResponseWriter, r *http.Request) {
	var input sub2APIUserPriceInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.Sub2APIUpstreamID <= 0 && strings.TrimSpace(input.AuthToken) == "" && (strings.TrimSpace(input.Email) == "" || strings.TrimSpace(input.Password) == "") {
		writeError(w, http.StatusBadRequest, "sub2api user username/password or auth token is required")
		return
	}
	result, err := s.inspectSub2APIUser(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (s *Server) fetchSub2APIUserFilterOptionsHandler(w http.ResponseWriter, r *http.Request) {
	var input sub2APIUserPriceInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.Sub2APIUpstreamID <= 0 {
		if strings.TrimSpace(input.BaseURL) == "" {
			writeError(w, http.StatusBadRequest, "sub2api upstream base url is required")
			return
		}
		if strings.TrimSpace(input.AuthToken) == "" && (strings.TrimSpace(input.Email) == "" || strings.TrimSpace(input.Password) == "") {
			writeError(w, http.StatusBadRequest, "sub2api user username/password or auth token is required")
			return
		}
	}
	result, err := s.fetchSub2APIUserFilterOptions(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (s *Server) fetchModelProbeHandler(w http.ResponseWriter, r *http.Request) {
	var input ModelProbeInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	input.APIKey = strings.TrimSpace(input.APIKey)
	var site Site
	var siteClient *NewAPIClient
	var siteUserID int64
	usingSavedSite := false
	resolvedSavedSiteToken := false
	if input.SiteID > 0 {
		var err error
		site, err = s.store.GetSite(r.Context(), input.SiteID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "未找到所选站点")
			return
		}
		usingSavedSite = true
		if input.APIKey == "" {
			siteClient, siteUserID, input.APIKey, err = s.newAPIClientForSite(r.Context(), site, false)
			if err != nil {
				writeError(w, http.StatusBadRequest, "该站点没有可用于探测的有效令牌："+err.Error())
				return
			}
			resolvedSavedSiteToken = true
		}
		if strings.TrimSpace(input.BaseURL) == "" {
			input.BaseURL = site.BaseURL
		}
	}
	if strings.TrimSpace(input.APIKey) == "" {
		writeError(w, http.StatusBadRequest, "API Key 不能为空，或者选择一个已保存且已登录的站点")
		return
	}
	result, err := FetchModelProbe(r.Context(), input)
	if err != nil && usingSavedSite && isModelProbeAuthError(err) {
		var refreshErr error
		siteClient, siteUserID, input.APIKey, refreshErr = s.newAPIClientForSite(r.Context(), site, true)
		if refreshErr == nil {
			resolvedSavedSiteToken = true
			s.saveNewAPISession(r.Context(), site, siteClient, siteUserID, input.APIKey, "")
			result, err = FetchModelProbe(r.Context(), input)
		} else {
			err = fmt.Errorf("已保存站点令牌不可用，重新登录刷新令牌也失败：%w", refreshErr)
		}
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if usingSavedSite && resolvedSavedSiteToken {
		s.saveNewAPISession(r.Context(), site, siteClient, siteUserID, input.APIKey, "")
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func isModelProbeAuthError(err error) bool {
	var httpErr modelProbeHTTPError
	return errors.As(err, &httpErr) && (httpErr.Status == http.StatusUnauthorized || httpErr.Status == http.StatusForbidden)
}

func (s *Server) upsertSub2APIAccount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AccountName string `json:"account_name"`
		APIURL      string `json:"apiurl"`
		APIKey      string `json:"api_key"`
		GroupName   string `json:"group_name"`
		GroupID     int64  `json:"group_id"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(input.APIKey) == "" {
		writeError(w, http.StatusBadRequest, "api key is required")
		return
	}
	client, err := s.sub2APIClient(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	group, err := client.EnsureGroupByIDOrName(r.Context(), input.GroupID, input.GroupName)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	account, action, err := client.UpsertOpenAIAPIKeyAccount(r.Context(), input.AccountName, input.APIURL, input.APIKey, group)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if input.Enabled != nil && !*input.Enabled {
		account, err = client.SetAccountEnabled(r.Context(), account.ID, false)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
	}
	s.notifySub2APIAccountUpdate(r.Context(), action, account)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"action":  action,
		"account": redactSub2Account(account),
	}})
}

func (s *Server) enableSub2APIAccount(w http.ResponseWriter, r *http.Request) {
	s.setSub2APIAccountEnabled(w, r, true)
}

func (s *Server) disableSub2APIAccount(w http.ResponseWriter, r *http.Request) {
	s.setSub2APIAccountEnabled(w, r, false)
}

func (s *Server) setSub2APIAccountEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid sub2api account id")
		return
	}
	client, err := s.sub2APIClient(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	account, err := client.SetAccountEnabled(r.Context(), id, enabled)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	action := "disabled"
	if enabled {
		action = "enabled"
	}
	s.notifySub2APIAccountUpdate(r.Context(), action, account)
	writeJSON(w, http.StatusOK, map[string]any{"data": redactSub2Account(account)})
}

func (s *Server) updateSub2APIAccountKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid sub2api account id")
		return
	}
	var input struct {
		APIURL    string `json:"apiurl"`
		APIKey    string `json:"api_key"`
		GroupName string `json:"group_name"`
		GroupID   int64  `json:"group_id"`
	}
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(input.APIKey) == "" {
		writeError(w, http.StatusBadRequest, "api key is required")
		return
	}
	client, err := s.sub2APIClient(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	var group sub2Group
	if input.GroupID > 0 || strings.TrimSpace(input.GroupName) != "" {
		group, err = client.EnsureGroupByIDOrName(r.Context(), input.GroupID, input.GroupName)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}
	account, err := client.UpdateAccountAPIKey(r.Context(), id, input.APIURL, input.APIKey, group)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.notifySub2APIAccountUpdate(r.Context(), "updated key", account)
	writeJSON(w, http.StatusOK, map[string]any{"data": redactSub2Account(account)})
}

func (s *Server) sub2APIClient(ctx context.Context, requireEnabled bool) (*Sub2APIClient, error) {
	settings, err := s.store.GetIntegrationSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load integration settings: %w", err)
	}
	if requireEnabled && !settings.Sub2APIEnabled {
		return nil, fmt.Errorf("sub2api sync is disabled")
	}
	baseURL := firstNonEmpty(settings.Sub2APIMainBaseURL, settings.Sub2APIBaseURL)
	adminKey := firstNonEmpty(settings.Sub2APIAdminKey, settings.Sub2APIAccessToken)
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("sub2api main base url is not configured")
	}
	if strings.TrimSpace(adminKey) == "" {
		return nil, fmt.Errorf("sub2api admin key is not configured")
	}
	client, err := NewSub2APIAdminClient(baseURL, adminKey)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (s *Server) sub2GroupFromRequest(ctx context.Context, client *Sub2APIClient, r *http.Request) (sub2Group, error) {
	groupID, _ := strconv.ParseInt(r.URL.Query().Get("group_id"), 10, 64)
	groupName := r.URL.Query().Get("group")
	if groupID <= 0 && strings.TrimSpace(groupName) == "" {
		return sub2Group{}, nil
	}
	return client.EnsureGroupByIDOrName(ctx, groupID, groupName)
}

func (s *Server) verifyNewAPISiteInput(ctx context.Context, input SiteInput, existing Site) (int64, string, string, string, error) {
	input = normalizeSiteInput(input)
	if input.Name == "" || input.BaseURL == "" || (input.AccessToken == "" && input.Username == "") || (input.AccessToken == "" && input.Password == "" && existing.Password == "") {
		return 0, "", "", "", fmt.Errorf("站点名称、地址必填，并且需要填写系统访问令牌，或填写用户名和密码")
	}
	client, err := NewNewAPIClient(input.BaseURL)
	if err != nil {
		return 0, "", "", "", err
	}
	if token := strings.TrimSpace(input.AccessToken); token != "" {
		userID, username, err := client.VerifySystemAccessToken(ctx, firstPositiveInt64(input.UserID, existing.UserID), token)
		if err != nil {
			return 0, "", "", client.DumpCookies(), fmt.Errorf("验证 NewAPI 系统访问令牌失败：%w", err)
		}
		return userID, token, firstNonEmpty(input.Username, username), client.DumpCookies(), nil
	}
	userID, err := client.Login(ctx, input.Username, input.Password, input.TOTPCode)
	if err != nil {
		return 0, "", "", client.DumpCookies(), fmt.Errorf("login NewAPI upstream: %w", err)
	}
	token, err := client.GenerateSystemAccessToken(ctx, userID)
	if err != nil {
		return userID, "", input.Username, client.DumpCookies(), fmt.Errorf("generate NewAPI system access token: %w", err)
	}
	return userID, token, input.Username, client.DumpCookies(), nil
}

func (s *Server) createSite(w http.ResponseWriter, r *http.Request) {
	var input SiteInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	input = normalizeSiteInput(input)
	if err := s.store.ensureUniqueSite(r.Context(), input, 0); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	userID, token, username, cookieJar, err := s.verifyNewAPISiteInput(r.Context(), input, Site{})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	input.Username = username
	site, err := s.store.CreateSite(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	runAt := time.Now()
	if err := s.store.UpdateSiteRunWithCookies(r.Context(), site.ID, userID, token, cookieJar, runAt, ""); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	site.UserID = userID
	site.AccessToken = token
	site.CookieJar = cookieJar
	site.LastError = ""
	site.LastRunAt = &runAt
	writeJSON(w, http.StatusCreated, map[string]any{"data": site})
}

func (s *Server) updateSite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid site id")
		return
	}
	var input SiteInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	input = normalizeSiteInput(input)
	existing, err := s.store.GetSite(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err := s.store.ensureUniqueSite(r.Context(), input, id); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	credentialsChanged := input.BaseURL != existing.BaseURL ||
		input.Username != existing.Username ||
		strings.TrimSpace(input.Password) != "" ||
		strings.TrimSpace(input.AccessToken) != "" ||
		input.TOTPCode != ""
	var userID int64
	var token string
	var username string
	var cookieJar string
	if credentialsChanged {
		verifyInput := input
		if strings.TrimSpace(verifyInput.Password) == "" {
			verifyInput.Password = existing.Password
		}
		userID, token, username, cookieJar, err = s.verifyNewAPISiteInput(r.Context(), verifyInput, existing)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		if strings.TrimSpace(input.Username) == "" {
			input.Username = username
		}
	}
	site, err := s.store.UpdateSite(r.Context(), id, input)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if notFound(err) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	if credentialsChanged {
		runAt := time.Now()
		if err := s.store.UpdateSiteRunWithCookies(r.Context(), site.ID, userID, token, cookieJar, runAt, ""); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		site.UserID = userID
		site.AccessToken = token
		site.CookieJar = cookieJar
		site.LastError = ""
		site.LastRunAt = &runAt
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": site})
}

func (s *Server) deleteSite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid site id")
		return
	}
	if err := s.store.DeleteSite(r.Context(), id); err != nil {
		status := http.StatusInternalServerError
		if notFound(err) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) refreshNewAPISiteTokens(w http.ResponseWriter, r *http.Request) {
	sites, err := s.store.ListSites(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取 NewAPI 站点列表失败")
		return
	}
	type tokenRefreshItem struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		BaseURL  string `json:"base_url"`
		Username string `json:"username"`
		UserID   int64  `json:"user_id,omitempty"`
		Success  bool   `json:"success"`
		Message  string `json:"message"`
	}
	result := struct {
		Total   int                `json:"total"`
		Success int                `json:"success"`
		Failed  int                `json:"failed"`
		Items   []tokenRefreshItem `json:"items"`
	}{Items: make([]tokenRefreshItem, 0, len(sites))}

	for _, site := range sites {
		result.Total++
		item := tokenRefreshItem{
			ID: site.ID, Name: site.Name, BaseURL: site.BaseURL, Username: site.Username,
		}
		runAt := time.Now()
		client, userID, token, refreshErr := s.newAPIClientForSite(r.Context(), site, true)
		if refreshErr != nil {
			message := localizeErrorText(refreshErr.Error())
			item.Success = false
			item.UserID = firstPositiveInt64(userID, site.UserID)
			item.Message = message
			result.Failed++
			if err := s.store.UpdateSiteRunWithCookies(r.Context(), site.ID, item.UserID, site.AccessToken, "", runAt, message); err != nil {
				item.Message = message + "；保存错误状态失败：" + localizeErrorText(err.Error())
			}
			result.Items = append(result.Items, item)
			continue
		}
		if err := s.store.UpdateSiteRunWithCookies(r.Context(), site.ID, userID, token, client.DumpCookies(), runAt, ""); err != nil {
			message := "系统访问令牌已获取，但写入数据库失败：" + localizeErrorText(err.Error())
			item.Success = false
			item.UserID = userID
			item.Message = message
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		item.Success = true
		item.UserID = userID
		item.Message = "系统访问令牌已获取并保存"
		result.Success++
		result.Items = append(result.Items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (s *Server) listCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := s.store.ListCategories(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list categories failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": categories})
}

func (s *Server) createCategory(w http.ResponseWriter, r *http.Request) {
	var input CategoryInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	category, err := s.store.CreateCategory(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": category})
}

func (s *Server) updateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid category id")
		return
	}
	var input CategoryInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	category, err := s.store.UpdateCategory(r.Context(), id, input)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if notFound(err) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": category})
}

func (s *Server) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid category id")
		return
	}
	if err := s.store.DeleteCategory(r.Context(), id); err != nil {
		status := http.StatusConflict
		if notFound(err) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.store.ListRules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list rules failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rules})
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request) {
	var input RuleInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rule, err := s.store.CreateRule(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": rule})
}

func (s *Server) bulkCreateClaudeRules(w http.ResponseWriter, r *http.Request) {
	input := bulkRuleInput{
		SourceType:      "newapi",
		Category:        "claud",
		ModelKeyword:    "claude-opus-4-8",
		ModelName:       "claude-opus-4-8",
		ScheduleEnabled: boolPtr(true),
		IntervalMinutes: 15,
	}
	s.bulkCreateRulesWithInput(w, r, input)
}

type bulkRuleInput struct {
	SourceType         string  `json:"source_type"`
	Category           string  `json:"category"`
	ModelKeyword       string  `json:"model_keyword"`
	ModelName          string  `json:"model_name"`
	GroupName          string  `json:"group_name"`
	ScheduleEnabled    *bool   `json:"schedule_enabled"`
	IntervalMinutes    int     `json:"interval_minutes"`
	SyncEnabled        *bool   `json:"sync_enabled"`
	SyncBaseGroup      string  `json:"sync_base_group"`
	SyncThresholdRatio float64 `json:"sync_threshold_ratio"`
}

func (s *Server) bulkCreateRules(w http.ResponseWriter, r *http.Request) {
	var input bulkRuleInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.bulkCreateRulesWithInput(w, r, input)
}

func (s *Server) bulkCreateRulesWithInput(w http.ResponseWriter, r *http.Request, input bulkRuleInput) {
	settings, err := s.store.GetIntegrationSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load settings failed")
		return
	}
	sites, err := s.store.ListSites(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list sites failed")
		return
	}
	upstreams, err := s.store.ListSub2APIUpstreams(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list sub2api upstreams failed")
		return
	}

	sourceType := strings.ToLower(strings.TrimSpace(input.SourceType))
	if sourceType == "" {
		sourceType = "all"
	}
	if sourceType != "all" && sourceType != RuleSourceNewAPI && sourceType != RuleSourceSub2API {
		writeError(w, http.StatusBadRequest, "source_type must be all, newapi or sub2api")
		return
	}
	categorySlug := normalizeCategorySlug(input.Category)
	if categorySlug == "" {
		writeError(w, http.StatusBadRequest, "category is required")
		return
	}
	modelKeyword := strings.TrimSpace(input.ModelKeyword)
	if modelKeyword == "" {
		modelKeyword = strings.TrimSpace(input.ModelName)
	}
	if modelKeyword == "" {
		writeError(w, http.StatusBadRequest, "model keyword is required")
		return
	}
	modelName := strings.TrimSpace(input.ModelName)
	if modelName == "" {
		modelName = modelKeyword
	}
	intervalMinutes := input.IntervalMinutes
	if intervalMinutes <= 0 {
		intervalMinutes = 15
	}
	roundIntervalMinutes, _ := normalizeMonitorScheduleSettings(settings.MonitorIntervalMinutes, settings.MonitorRuleDelaySeconds)
	if roundIntervalMinutes <= 0 {
		roundIntervalMinutes = intervalMinutes
	}
	scheduleEnabled := true
	if input.ScheduleEnabled != nil {
		scheduleEnabled = *input.ScheduleEnabled
	}
	syncEnabled := true
	if input.SyncEnabled != nil {
		syncEnabled = *input.SyncEnabled
	}
	syncThresholdRatio := input.SyncThresholdRatio
	if syncThresholdRatio <= 0 {
		if thresholdRatio := syncThresholdRatioForCategory(settings, categorySlug); thresholdRatio != nil {
			syncThresholdRatio = *thresholdRatio
		}
	}

	totalTargets := 0
	if sourceType == "all" || sourceType == RuleSourceNewAPI {
		totalTargets += len(sites)
	}
	if sourceType == "all" || sourceType == RuleSourceSub2API {
		totalTargets += len(upstreams)
	}
	created := make([]Rule, 0, totalTargets)
	skipped := 0
	targetIndex := 0
	nextInitialRunAt := func() *time.Time {
		if !scheduleEnabled {
			return nil
		}
		offset := staggerOffset(targetIndex, totalTargets, roundIntervalMinutes)
		targetIndex++
		next := time.Now().Add(offset)
		return &next
	}
	create := func(ruleInput RuleInput) bool {
		rule, createErr := s.store.CreateRule(r.Context(), ruleInput)
		if createErr != nil {
			if isDuplicateRuleErr(createErr) {
				skipped++
				return true
			}
			writeError(w, http.StatusUnprocessableEntity, createErr.Error())
			return false
		}
		created = append(created, rule)
		return true
	}
	if sourceType == "all" || sourceType == RuleSourceNewAPI {
		for _, site := range sites {
			if !create(RuleInput{
				SourceType:         RuleSourceNewAPI,
				SiteID:             site.ID,
				Category:           categorySlug,
				ModelKeyword:       modelKeyword,
				ModelName:          modelName,
				GroupName:          input.GroupName,
				Enabled:            true,
				ScheduleEnabled:    scheduleEnabled,
				IntervalMinutes:    intervalMinutes,
				InitialNextRunAt:   nextInitialRunAt(),
				SyncEnabled:        syncEnabled,
				SyncBaseGroup:      input.SyncBaseGroup,
				SyncThresholdRatio: syncThresholdRatio,
			}) {
				return
			}
		}
	}
	if sourceType == "all" || sourceType == RuleSourceSub2API {
		for _, upstream := range upstreams {
			if !create(RuleInput{
				SourceType:         RuleSourceSub2API,
				Sub2APIUpstreamID:  upstream.ID,
				Category:           categorySlug,
				ModelKeyword:       modelKeyword,
				ModelName:          modelName,
				GroupName:          input.GroupName,
				Enabled:            true,
				ScheduleEnabled:    scheduleEnabled,
				IntervalMinutes:    intervalMinutes,
				InitialNextRunAt:   nextInitialRunAt(),
				SyncEnabled:        syncEnabled,
				SyncBaseGroup:      input.SyncBaseGroup,
				SyncThresholdRatio: syncThresholdRatio,
			}) {
				return
			}
		}
	}
	staggered, err := s.store.StaggerRules(r.Context(), sourceType, categorySlug, modelKeyword, roundIntervalMinutes, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "stagger rules failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"created":         len(created),
		"skipped":         skipped,
		"staggered":       staggered,
		"total_targets":   totalTargets,
		"total_sites":     len(sites),
		"total_upstreams": len(upstreams),
		"source_type":     sourceType,
		"category":        categorySlug,
		"model_keyword":   modelKeyword,
		"model_name":      modelName,
		"sync_enabled":    syncEnabled,
		"rules":           created,
	}})
}

func staggerOffset(index int, total int, intervalMinutes int) time.Duration {
	if total <= 1 {
		return time.Minute
	}
	if intervalMinutes <= 0 {
		intervalMinutes = 15
	}
	window := time.Duration(intervalMinutes) * time.Minute
	step := window / time.Duration(total)
	if step < time.Minute {
		step = time.Minute
	}
	return time.Minute + time.Duration(index)*step
}

func isDuplicateRuleErr(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "已存在") ||
		strings.Contains(text, "duplicate") ||
		strings.Contains(text, "repeat")
}

func boolPtr(value bool) *bool {
	return &value
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (s *Server) bulkUpdateRules(w http.ResponseWriter, r *http.Request) {
	var input BulkRuleUpdateInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rules, err := s.store.BulkUpdateRules(r.Context(), input)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if notFound(err) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"updated": len(rules),
		"rules":   rules,
	}})
}

func (s *Server) updateRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	var input RuleInput
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rule, err := s.store.UpdateRule(r.Context(), id, input)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if notFound(err) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rule})
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	if err := s.store.DeleteRule(r.Context(), id); err != nil {
		status := http.StatusInternalServerError
		if notFound(err) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) runRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	snapshots, err := s.RunRule(r.Context(), id)
	if err != nil {
		status := http.StatusBadGateway
		if notFound(err) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	if err := s.store.RestoreRuleAfterManualRun(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "restore rule after manual run failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"count":     len(snapshots),
		"snapshots": snapshots,
	}})
}

func (s *Server) runRulePoll(w http.ResponseWriter, r *http.Request) {
	if !s.startRulePoll() {
		writeError(w, http.StatusConflict, "已有一轮全局轮询正在执行")
		return
	}
	go func() {
		defer s.finishRulePoll()
		s.runEnabledRules(context.Background())
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"data": map[string]any{
		"status":  "started",
		"message": "已开始执行一轮全局轮询",
	}})
}

func (s *Server) startRulePoll() bool {
	s.pollMu.Lock()
	defer s.pollMu.Unlock()
	if s.pollRunning {
		return false
	}
	s.pollRunning = true
	return true
}

func (s *Server) finishRulePoll() {
	s.pollMu.Lock()
	s.pollRunning = false
	s.pollMu.Unlock()
}

func (s *Server) runEnabledRulesIfIdle(ctx context.Context) bool {
	if !s.startRulePoll() {
		return false
	}
	defer s.finishRulePoll()
	s.runEnabledRules(ctx)
	return true
}

func (s *Server) listSnapshots(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	settings, err := s.store.GetIntegrationSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list snapshots failed")
		return
	}
	snapshots, err := s.store.LatestSnapshots(r.Context(), limit, r.URL.Query().Get("category"), settings.ExpectedCacheHitRatio, effectiveLatencyWeight(settings))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list snapshots failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": snapshots})
}

func (s *Server) RunRule(ctx context.Context, ruleID int64) ([]PriceSnapshot, error) {
	rule, site, upstream, err := s.store.GetRuleWithSource(ctx, ruleID)
	if err != nil {
		return nil, err
	}
	return s.runRuleWithSource(ctx, rule, site, upstream)
}

func (s *Server) runRuleWithoutSync(ctx context.Context, ruleID int64) ([]PriceSnapshot, error) {
	rule, site, upstream, err := s.store.GetRuleWithSource(ctx, ruleID)
	if err != nil {
		return nil, err
	}
	rule.SyncEnabled = false
	return s.runRuleWithSource(ctx, rule, site, upstream)
}

func (s *Server) runRuleWithSource(ctx context.Context, rule Rule, site Site, upstream Sub2APIUpstream) ([]PriceSnapshot, error) {
	switch strings.ToLower(strings.TrimSpace(rule.SourceType)) {
	case "", RuleSourceNewAPI:
		return s.runNewAPIRule(ctx, rule, site)
	case RuleSourceSub2API:
		return s.runSub2APIRule(ctx, rule, upstream)
	default:
		return nil, fmt.Errorf("unsupported rule source type %q", rule.SourceType)
	}
}

func (s *Server) runNewAPIRule(ctx context.Context, rule Rule, site Site) ([]PriceSnapshot, error) {
	settings, err := s.store.GetIntegrationSettings(ctx)
	if err != nil {
		return nil, err
	}
	expectedCacheHitRatio := settings.ExpectedCacheHitRatio
	latencyWeight := effectiveLatencyWeight(settings)
	client, userID, token, err := s.newAPIClientForSite(ctx, site, false)
	if err != nil {
		s.saveNewAPISession(ctx, site, client, site.UserID, site.AccessToken, err.Error())
		return nil, err
	}

	pricing, _, err := client.FetchPricing(ctx, userID, token)
	if err != nil {
		if generated, tokenErr := client.GenerateSystemAccessToken(ctx, userID); tokenErr == nil {
			token = generated
			pricing, _, err = client.FetchPricing(ctx, userID, token)
		}
	}
	if err != nil {
		if isSessionAuthError(err) {
			client, userID, token, err = s.newAPIClientForSite(ctx, site, true)
			if err == nil {
				pricing, _, err = client.FetchPricing(ctx, userID, token)
			}
		}
	}
	if err != nil {
		s.saveNewAPISession(ctx, site, client, userID, token, err.Error())
		return nil, err
	}
	if groupPricing, groupErr := client.FetchUserSelfGroups(ctx, userID, token); groupErr == nil {
		ApplyNewAPIUserGroupPricing(pricing, groupPricing)
	} else {
		log.Printf("fetch newapi user groups for site %d: %v", site.ID, groupErr)
	}
	s.recordNewAPIRuleCheckin(ctx, rule.ID, site.ID, client, userID, token)
	balance, balanceErr := client.FetchBalance(ctx, userID, token)
	if balanceErr != nil {
		log.Printf("fetch newapi balance for site %d: %v", site.ID, balanceErr)
		balance.Unit = "usd"
	}
	rechargeStatus, rechargeErr := client.FetchRechargeStatus(ctx, userID, token)
	if rechargeErr != nil {
		log.Printf("fetch newapi recharge status for site %d: %v", site.ID, rechargeErr)
	}

	rows, err := BuildKeywordRowsWithExpectedCacheHitRatio(pricing, rule.ModelKeyword, expectedCacheHitRatio)
	if err != nil {
		s.saveNewAPISession(ctx, site, client, userID, token, err.Error())
		return nil, err
	}
	rows = filterPricingRowsForRule(rule, rows)
	beforeBlockedFilter := len(rows)
	rows = filterPricingRowsByIncludedKeywords(rows, categorySub2APIMainGroupKeywords(ctx, s.store, rule.Category))
	rows = filterPricingRowsByBlockedKeywords(rows, categoryBlockedGroupKeywords(ctx, s.store, rule.Category))
	lowPriceGroupLimit := normalizeSub2APISyncKeepCount(settings.Sub2APISyncKeepCount)
	rows = CheapestPricingRowsWithExpectedCacheHitRatioLimit(rows, expectedCacheHitRatio, lowPriceGroupLimit)
	if len(rows) == 0 {
		err := missingPricingRowsError(rule, false, beforeBlockedFilter > 0)
		_ = s.store.MarkMissingSnapshotGroupsInvalid(ctx, rule.ID, rule.ModelKeyword, nil, missingSnapshotInvalidReason(beforeBlockedFilter > 0))
		s.saveNewAPISession(ctx, site, client, userID, token, err.Error())
		return nil, err
	}

	snapshots := make([]PriceSnapshot, 0, len(rows))
	activeGroups := make([]string, 0, len(rows))
	var syncErr error
	syncAttempted := false
	syncDecisionRecorded := false
	for _, row := range rows {
		activeGroups = append(activeGroups, row.GroupName)
		previousLowest, previousLowestErr := s.store.CheapestLatestSnapshot(ctx, rule.Category, row.ModelName, expectedCacheHitRatio, latencyWeight)
		requestLatencyMS := s.measureNewAPIUpstreamLatency(ctx, rule, site, client, userID, token, row)
		snapshot := PriceSnapshot{
			RuleID:             rule.ID,
			SourceType:         RuleSourceNewAPI,
			SiteID:             site.ID,
			SiteName:           site.Name,
			SiteBaseURL:        site.BaseURL,
			SourceAccount:      sourceAccountForSite(site),
			Category:           rule.Category,
			CategoryName:       rule.CategoryName,
			ModelKeyword:       rule.ModelKeyword,
			ModelName:          row.ModelName,
			GroupName:          row.GroupName,
			GroupDesc:          row.GroupDesc,
			QuotaType:          row.QuotaType,
			GroupRatio:         ptr(row.GroupRatio),
			InputPrice:         row.InputPrice,
			OutputPrice:        row.OutputPrice,
			CacheReadPrice:     row.CacheReadPrice,
			CacheWritePrice:    row.CacheWritePrice,
			RequestPrice:       row.RequestPrice,
			RequestLatencyMS:   requestLatencyMS,
			UpstreamBalance:    balance.Value,
			BalanceUnit:        balance.Unit,
			OnlineTopupEnabled: rechargeStatus.Enabled,
			RechargeMultiplier: rechargeStatus.Multiplier,
			Raw:                PricingRowRaw(row),
		}
		snapshot, err = s.store.InsertSnapshot(ctx, snapshot)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
		currentLowest, newLowest, stableLowest := s.lowestSnapshotEvent(ctx, rule, snapshot, previousLowest, previousLowestErr, expectedCacheHitRatio, latencyWeight)
		if stableLowest {
			syncDecisionRecorded = true
		}
		if newLowest {
			s.notifyPriceChange(ctx, previousLowest, currentLowest, lowestSnapshotChanges(previousLowest, currentLowest))
		}
		if rule.SyncEnabled {
			attempted, decisionRecorded, err := s.syncBestAvailableCandidate(ctx, rule, row.ModelName, expectedCacheHitRatio, latencyWeight)
			syncDecisionRecorded = syncDecisionRecorded || decisionRecorded
			if attempted {
				syncAttempted = true
			}
			if err != nil {
				syncErr = err
			}
		}
	}
	if err := s.store.MarkMissingSnapshotGroupsInvalid(ctx, rule.ID, rule.ModelKeyword, activeGroups, "upstream group disappeared or is no longer cheapest"); err != nil {
		log.Printf("mark missing newapi snapshot groups invalid for rule %d: %v", rule.ID, err)
	}
	s.saveNewAPISession(ctx, site, client, userID, token, "")
	if rule.SyncEnabled && !syncAttempted && syncErr == nil && !syncDecisionRecorded {
		_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, "不是当前最低价", "")
	}
	return snapshots, nil
}

func (s *Server) runSub2APIRule(ctx context.Context, rule Rule, upstream Sub2APIUpstream) ([]PriceSnapshot, error) {
	if upstream.ID <= 0 {
		return nil, fmt.Errorf("sub2api source site is required")
	}
	settings, err := s.store.GetIntegrationSettings(ctx)
	if err != nil {
		return nil, err
	}
	expectedCacheHitRatio := settings.ExpectedCacheHitRatio
	latencyWeight := effectiveLatencyWeight(settings)
	client, groups, userRates, err := s.fetchSub2APIUserClientGroups(ctx, sub2APIUserPriceInput{
		Sub2APIUpstreamID: upstream.ID,
	})
	if err != nil {
		return nil, err
	}
	s.recordSub2APIRuleCheckin(ctx, rule.ID, upstream.ID, client)
	balance, balanceErr := client.FetchBalance(ctx)
	if balanceErr != nil {
		log.Printf("fetch sub2api balance for upstream %d: %v", upstream.ID, balanceErr)
		balance.Unit = "usd"
	}
	rechargeStatus, rechargeErr := client.FetchRechargeStatus(ctx)
	if rechargeErr != nil {
		log.Printf("fetch sub2api recharge status for upstream %d: %v", upstream.ID, rechargeErr)
	}
	priceURL := defaultOfficialPriceURL
	officialPrices, _, err := loadOfficialPrices(ctx, priceURL)
	if err != nil {
		return nil, err
	}
	result := sub2APIUserPriceResult{
		Groups:         groups,
		UserGroupRates: userRates,
		Rows: buildSub2APIUserPriceRows(officialPrices, groups, userRates, sub2APIUserPriceInput{
			Sub2APIUpstreamID: upstream.ID,
			Models:            rule.ModelKeyword,
			Modes:             "chat,responses,image_generation",
			Limit:             5000,
		}),
	}
	rows := filterSub2APIPriceRowsForRule(rule, result.Rows)
	beforeBlockedFilter := len(rows)
	rows = filterSub2APIPriceRowsByIncludedKeywords(rows, categorySub2APIMainGroupKeywords(ctx, s.store, rule.Category))
	rows = filterSub2APIPriceRowsByBlockedKeywords(rows, categoryBlockedGroupKeywords(ctx, s.store, rule.Category))
	lowPriceGroupLimit := normalizeSub2APISyncKeepCount(settings.Sub2APISyncKeepCount)
	rows = cheapestSub2PriceRowsWithExpectedCacheHitRatioLimit(rows, expectedCacheHitRatio, lowPriceGroupLimit)
	if len(rows) == 0 {
		_ = s.store.MarkMissingSnapshotGroupsInvalid(ctx, rule.ID, rule.ModelKeyword, nil, missingSnapshotInvalidReason(beforeBlockedFilter > 0))
		return nil, missingPricingRowsError(rule, true, beforeBlockedFilter > 0)
	}

	snapshots := make([]PriceSnapshot, 0, len(rows))
	activeGroups := make([]string, 0, len(rows))
	var syncErr error
	syncAttempted := false
	syncDecisionRecorded := false
	for _, row := range rows {
		activeGroups = append(activeGroups, row.GroupName)
		previousLowest, previousLowestErr := s.store.CheapestLatestSnapshot(ctx, rule.Category, row.ModelName, expectedCacheHitRatio, latencyWeight)
		requestLatencyMS := s.measureSub2APIUpstreamLatency(ctx, rule, upstream, row)
		cacheWritePrice := row.FinalCacheWritePerMillion
		if cacheWritePrice == nil {
			cacheWritePrice = row.FinalCacheWrite1hPerMillion
		}
		snapshot := PriceSnapshot{
			RuleID:             rule.ID,
			SourceType:         RuleSourceSub2API,
			Sub2APIUpstreamID:  upstream.ID,
			SiteName:           upstream.Name,
			SiteBaseURL:        upstream.BaseURL,
			SourceAccount:      sourceAccountForSub2APIUpstream(upstream),
			Category:           rule.Category,
			CategoryName:       rule.CategoryName,
			ModelKeyword:       rule.ModelKeyword,
			ModelName:          row.ModelName,
			GroupName:          row.GroupName,
			GroupDesc:          strings.TrimSpace(row.GroupPlatform),
			QuotaType:          0,
			GroupRatio:         ptr(row.EffectiveRate),
			InputPrice:         row.FinalInputPerMillion,
			OutputPrice:        row.FinalOutputPerMillion,
			CacheReadPrice:     row.FinalCacheReadPerMillion,
			CacheWritePrice:    cacheWritePrice,
			RequestLatencyMS:   requestLatencyMS,
			UpstreamBalance:    balance.Value,
			BalanceUnit:        balance.Unit,
			OnlineTopupEnabled: rechargeStatus.Enabled,
			RechargeMultiplier: rechargeStatus.Multiplier,
			Raw:                sub2APIUserPriceRowRaw(row),
		}
		snapshot, err = s.store.InsertSnapshot(ctx, snapshot)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
		currentLowest, newLowest, stableLowest := s.lowestSnapshotEvent(ctx, rule, snapshot, previousLowest, previousLowestErr, expectedCacheHitRatio, latencyWeight)
		if stableLowest {
			syncDecisionRecorded = true
		}
		if newLowest {
			s.notifyPriceChange(ctx, previousLowest, currentLowest, lowestSnapshotChanges(previousLowest, currentLowest))
		}
		if rule.SyncEnabled {
			attempted, decisionRecorded, err := s.syncBestAvailableCandidate(ctx, rule, row.ModelName, expectedCacheHitRatio, latencyWeight)
			syncDecisionRecorded = syncDecisionRecorded || decisionRecorded
			if attempted {
				syncAttempted = true
			}
			if err != nil {
				syncErr = err
			}
		}
	}
	if err := s.store.MarkMissingSnapshotGroupsInvalid(ctx, rule.ID, rule.ModelKeyword, activeGroups, "upstream group disappeared or is no longer cheapest"); err != nil {
		log.Printf("mark missing sub2api snapshot groups invalid for rule %d: %v", rule.ID, err)
	}
	if rule.SyncEnabled && !syncAttempted && syncErr == nil && !syncDecisionRecorded {
		_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, "不是当前最低价", "")
	}
	return snapshots, nil
}

func (s *Server) recordNewAPIRuleCheckin(ctx context.Context, ruleID, siteID int64, client *NewAPIClient, userID int64, token string) {
	result, err := client.EnsureDailyCheckin(ctx, userID, token, time.Now())
	if err != nil {
		result = CheckinResult{
			Enabled:   true,
			Status:    "failed",
			Unit:      "usd",
			Message:   err.Error(),
			CheckedAt: time.Now(),
		}
	}
	if err := s.store.UpdateRuleCheckinStatus(ctx, ruleID, result); err != nil {
		log.Printf("update newapi checkin status for rule %d site %d: %v", ruleID, siteID, err)
	}
}

func (s *Server) recordSub2APIRuleCheckin(ctx context.Context, ruleID, upstreamID int64, client *Sub2APIClient) {
	result, err := client.EnsureDailyCheckin(ctx, time.Now())
	if err != nil {
		result = CheckinResult{
			Enabled:   false,
			Status:    "disabled",
			Unit:      "usd",
			Message:   err.Error(),
			CheckedAt: time.Now(),
		}
	}
	if err := s.store.UpdateRuleCheckinStatus(ctx, ruleID, result); err != nil {
		log.Printf("update sub2api checkin status for rule %d upstream %d: %v", ruleID, upstreamID, err)
	}
}

func pricingRowFromSub2APIUserPriceRow(row Sub2APIUserPriceRow) PricingRow {
	cacheWritePrice := row.FinalCacheWritePerMillion
	if cacheWritePrice == nil {
		cacheWritePrice = row.FinalCacheWrite1hPerMillion
	}
	return PricingRow{
		ModelName:       row.ModelName,
		GroupName:       row.GroupName,
		GroupDesc:       strings.TrimSpace(row.GroupPlatform),
		QuotaType:       0,
		GroupRatio:      row.EffectiveRate,
		InputPrice:      row.FinalInputPerMillion,
		OutputPrice:     row.FinalOutputPerMillion,
		CacheReadPrice:  row.FinalCacheReadPerMillion,
		CacheWritePrice: cacheWritePrice,
	}
}

func (s *Server) measureNewAPIUpstreamLatency(ctx context.Context, rule Rule, site Site, client *NewAPIClient, userID int64, token string, row PricingRow) *float64 {
	settings, err := s.store.GetIntegrationSettings(ctx)
	if err != nil || !settings.LatencyTestEnabled {
		return nil
	}
	keyName := upstreamKeyName(rule, row.ModelName)
	apiKey, _, err := createNewAPIUpstreamKey(ctx, client, userID, token, keyName, row.GroupName)
	if err != nil && isSessionAuthError(err) {
		client, userID, token, err = s.newAPIClientForSite(ctx, site, true)
		if err == nil {
			apiKey, _, err = createNewAPIUpstreamKey(ctx, client, userID, token, keyName, row.GroupName)
			if err == nil {
				s.saveNewAPISession(ctx, site, client, userID, token, "")
			}
		}
	}
	if err != nil {
		log.Printf("create newapi latency probe key for rule %d group %q: %v", rule.ID, row.GroupName, err)
		return nil
	}
	latency, err := probeModelRequestLatency(ctx, site.BaseURL, apiKey, row.ModelName)
	if err != nil {
		log.Printf("probe newapi latency for rule %d site %s group %q model %q: %v", rule.ID, site.Name, row.GroupName, row.ModelName, err)
		return nil
	}
	return ptr(latency)
}

func (s *Server) measureSub2APIUpstreamLatency(ctx context.Context, rule Rule, upstream Sub2APIUpstream, row Sub2APIUserPriceRow) *float64 {
	settings, err := s.store.GetIntegrationSettings(ctx)
	if err != nil || !settings.LatencyTestEnabled {
		return nil
	}
	group := sub2Group{
		ID:       row.GroupID,
		Name:     row.GroupName,
		Platform: row.GroupPlatform,
		Rate:     row.EffectiveRate,
	}
	apiKey, _, err := s.ensureSub2APIUpstreamAPIKey(ctx, upstream, upstreamKeyName(rule, row.ModelName), group)
	if err != nil {
		log.Printf("create sub2api latency probe key for rule %d group %q: %v", rule.ID, row.GroupName, err)
		return nil
	}
	latency, err := probeModelRequestLatency(ctx, upstream.BaseURL, apiKey, row.ModelName)
	if err != nil {
		log.Printf("probe sub2api latency for rule %d upstream %s group %q model %q: %v", rule.ID, upstream.Name, row.GroupName, row.ModelName, err)
		return nil
	}
	return ptr(latency)
}

func missingPricingRowsError(rule Rule, sub2api bool, blockedOnly bool) error {
	source := "上游价格接口"
	if sub2api {
		source = "sub2api 上游价格接口"
	}
	if blockedOnly {
		return fmt.Errorf("未找到可用低价分组：模型 %q，分类 %q。已抓取到候选分组，但全部命中该分类设置的屏蔽关键词",
			rule.ModelKeyword,
			firstNonEmpty(rule.CategoryName, rule.Category),
		)
	}
	return fmt.Errorf("未找到模型价格：模型 %q，分类 %q。可能原因：%s 没有返回该模型，或该模型不属于当前分类分组",
		rule.ModelKeyword,
		firstNonEmpty(rule.CategoryName, rule.Category),
		source,
	)
}

func missingSnapshotInvalidReason(blockedOnly bool) string {
	if blockedOnly {
		return "分组命中当前分类屏蔽关键词，已排除低价候选"
	}
	return "上游分组不匹配当前规则分类，或模型/分组已不存在"
}

func cheapestSub2PriceRows(rows []Sub2APIUserPriceRow) []Sub2APIUserPriceRow {
	return cheapestSub2PriceRowsWithExpectedCacheHitRatio(rows, 0)
}

func cheapestSub2PriceRowsWithExpectedCacheHitRatio(rows []Sub2APIUserPriceRow, expectedCacheHitRatio float64) []Sub2APIUserPriceRow {
	return cheapestSub2PriceRowsWithExpectedCacheHitRatioLimit(rows, expectedCacheHitRatio, 1)
}

func cheapestSub2PriceRowsWithExpectedCacheHitRatioLimit(rows []Sub2APIUserPriceRow, expectedCacheHitRatio float64, limitPerModel int) []Sub2APIUserPriceRow {
	limitPerModel = normalizeSub2APISyncKeepCount(limitPerModel)
	grouped := map[string][]Sub2APIUserPriceRow{}
	for _, row := range rows {
		model := strings.TrimSpace(row.ModelName)
		if model == "" {
			continue
		}
		grouped[model] = append(grouped[model], row)
	}
	models := make([]string, 0, len(grouped))
	for model := range grouped {
		models = append(models, model)
	}
	sort.Strings(models)
	out := make([]Sub2APIUserPriceRow, 0, len(models)*limitPerModel)
	for _, model := range models {
		modelRows := grouped[model]
		sort.SliceStable(modelRows, func(i, j int) bool {
			if sub2APIUserPriceRowLessWithExpectedCacheHitRatio(modelRows[i], modelRows[j], expectedCacheHitRatio) {
				return true
			}
			if sub2APIUserPriceRowLessWithExpectedCacheHitRatio(modelRows[j], modelRows[i], expectedCacheHitRatio) {
				return false
			}
			return modelRows[i].GroupName < modelRows[j].GroupName
		})
		if len(modelRows) > limitPerModel {
			modelRows = modelRows[:limitPerModel]
		}
		out = append(out, modelRows...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if sub2APIUserPriceRowLessWithExpectedCacheHitRatio(out[i], out[j], expectedCacheHitRatio) {
			return true
		}
		if sub2APIUserPriceRowLessWithExpectedCacheHitRatio(out[j], out[i], expectedCacheHitRatio) {
			return false
		}
		if out[i].ModelName != out[j].ModelName {
			return out[i].ModelName < out[j].ModelName
		}
		return out[i].GroupName < out[j].GroupName
	})
	return out
}

func sub2APIUserPriceRowLess(left, right Sub2APIUserPriceRow) bool {
	leftPrice := nullablePriceValue(left.FinalInputPerMillion, left.FinalOutputPerMillion, left.FinalCacheReadPerMillion)
	rightPrice := nullablePriceValue(right.FinalInputPerMillion, right.FinalOutputPerMillion, right.FinalCacheReadPerMillion)
	if leftPrice != rightPrice {
		return leftPrice < rightPrice
	}
	leftOutput := nullablePriceValue(left.FinalOutputPerMillion)
	rightOutput := nullablePriceValue(right.FinalOutputPerMillion)
	if leftOutput != rightOutput {
		return leftOutput < rightOutput
	}
	if left.EffectiveRate != right.EffectiveRate {
		return left.EffectiveRate < right.EffectiveRate
	}
	return left.GroupName < right.GroupName
}

func sub2APIUserPriceRowLessWithExpectedCacheHitRatio(left, right Sub2APIUserPriceRow, expectedCacheHitRatio float64) bool {
	leftPrice := sub2APIUserPriceRowExpectedPrice(left, expectedCacheHitRatio)
	rightPrice := sub2APIUserPriceRowExpectedPrice(right, expectedCacheHitRatio)
	if leftPrice != rightPrice {
		return leftPrice < rightPrice
	}
	leftOutput := nullablePriceValue(left.FinalOutputPerMillion)
	rightOutput := nullablePriceValue(right.FinalOutputPerMillion)
	if leftOutput != rightOutput {
		return leftOutput < rightOutput
	}
	if left.EffectiveRate != right.EffectiveRate {
		return left.EffectiveRate < right.EffectiveRate
	}
	return left.GroupName < right.GroupName
}

func sub2APIUserPriceRowExpectedPrice(row Sub2APIUserPriceRow, expectedCacheHitRatio float64) float64 {
	hitRatio := normalizeExpectedCacheHitRatio(expectedCacheHitRatio)
	if row.FinalCacheReadPerMillion == nil && row.FinalCacheWritePerMillion == nil && row.FinalCacheWrite1hPerMillion == nil {
		return sub2APIUserPriceRowBasePrice(row) * (1 + hitRatio)
	}
	missPrice := firstComparablePrice(row.FinalCacheWritePerMillion, row.FinalCacheWrite1hPerMillion, row.FinalInputPerMillion, row.FinalOutputPerMillion)
	hitPrice := firstComparablePrice(row.FinalCacheReadPerMillion, row.FinalInputPerMillion, row.FinalOutputPerMillion)
	if missPrice == 1e308 && hitPrice == 1e308 {
		return 1e308
	}
	expected := missPrice*(1-hitRatio) + hitPrice*hitRatio
	if row.FinalInputPerMillion != nil && row.FinalOutputPerMillion != nil {
		expected += *row.FinalOutputPerMillion
	}
	return expected
}

func sub2APIUserPriceRowBasePrice(row Sub2APIUserPriceRow) float64 {
	if row.FinalInputPerMillion != nil {
		price := *row.FinalInputPerMillion
		if row.FinalOutputPerMillion != nil {
			price += *row.FinalOutputPerMillion
		}
		return price
	}
	if row.FinalOutputPerMillion != nil {
		return *row.FinalOutputPerMillion
	}
	return 1e308
}

type pricingCategoryKind int

const (
	pricingCategoryUnknown pricingCategoryKind = iota
	pricingCategoryOpenAI
	pricingCategoryClaude
)

func filterPricingRowsForRule(rule Rule, rows []PricingRow) []PricingRow {
	kind := pricingCategoryKindForRule(rule)
	if kind == pricingCategoryUnknown {
		return rows
	}
	filtered := make([]PricingRow, 0, len(rows))
	for _, row := range rows {
		if pricingRowMatchesCategoryKind(row, kind) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func filterSub2APIPriceRowsForRule(rule Rule, rows []Sub2APIUserPriceRow) []Sub2APIUserPriceRow {
	kind := pricingCategoryKindForRule(rule)
	if kind == pricingCategoryUnknown {
		return rows
	}
	filtered := make([]Sub2APIUserPriceRow, 0, len(rows))
	for _, row := range rows {
		platform := strings.ToLower(strings.TrimSpace(row.GroupPlatform))
		switch kind {
		case pricingCategoryClaude:
			if platform == sub2PlatformAnthropic || platform == "claude" || platform == "claud" {
				filtered = append(filtered, row)
			}
		case pricingCategoryOpenAI:
			if platform == sub2PlatformOpenAI || platform == "codex" || platform == "gpt" {
				filtered = append(filtered, row)
			}
		}
	}
	return filtered
}

func categoryBlockedGroupKeywords(ctx context.Context, store Store, categorySlug string) []string {
	category, err := store.GetCategoryBySlug(ctx, categorySlug)
	if err != nil {
		if !notFound(err) {
			log.Printf("load category blocked keywords %q: %v", categorySlug, err)
		}
		return nil
	}
	return category.BlockedGroupKeywords
}

func categorySub2APIMainGroupKeywords(ctx context.Context, store Store, categorySlug string) []string {
	category, err := store.GetCategoryBySlug(ctx, categorySlug)
	if err != nil {
		if !notFound(err) {
			log.Printf("load category included keywords %q: %v", categorySlug, err)
		}
		return nil
	}
	return category.Sub2APIMainGroupKeywords
}

func filterPricingRowsByIncludedKeywords(rows []PricingRow, keywords []string) []PricingRow {
	if len(keywords) == 0 || len(rows) == 0 {
		return rows
	}
	filtered := make([]PricingRow, 0, len(rows))
	for _, row := range rows {
		if groupMatchesIncludedKeywords(row.GroupName, row.GroupDesc, keywords) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func filterSub2APIPriceRowsByIncludedKeywords(rows []Sub2APIUserPriceRow, keywords []string) []Sub2APIUserPriceRow {
	if len(keywords) == 0 || len(rows) == 0 {
		return rows
	}
	filtered := make([]Sub2APIUserPriceRow, 0, len(rows))
	for _, row := range rows {
		if groupMatchesIncludedKeywords(row.GroupName, row.GroupPlatform, keywords) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func filterPricingRowsByBlockedKeywords(rows []PricingRow, keywords []string) []PricingRow {
	if len(keywords) == 0 || len(rows) == 0 {
		return rows
	}
	filtered := make([]PricingRow, 0, len(rows))
	for _, row := range rows {
		if groupMatchesBlockedKeywords(row.GroupName, row.GroupDesc, keywords) {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func filterSub2APIPriceRowsByBlockedKeywords(rows []Sub2APIUserPriceRow, keywords []string) []Sub2APIUserPriceRow {
	if len(keywords) == 0 || len(rows) == 0 {
		return rows
	}
	filtered := make([]Sub2APIUserPriceRow, 0, len(rows))
	for _, row := range rows {
		if groupMatchesBlockedKeywords(row.GroupName, row.GroupPlatform, keywords) {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func groupMatchesBlockedKeywords(groupName string, groupDesc string, keywords []string) bool {
	text := strings.ToLower(strings.TrimSpace(groupName + " " + groupDesc))
	if text == "" {
		return false
	}
	for _, keyword := range keywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" && strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func groupMatchesIncludedKeywords(groupName string, groupDesc string, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(groupName + " " + groupDesc))
	if text == "" {
		return false
	}
	for _, keyword := range keywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" && strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func pricingCategoryKindForRule(rule Rule) pricingCategoryKind {
	value := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		rule.Category,
		rule.CategoryName,
		rule.ModelKeyword,
		rule.ModelName,
	}, " ")))
	if containsAny(value, "claude", "claud", "anthropic") {
		return pricingCategoryClaude
	}
	if containsAny(value, "codex", "openai", "gpt") {
		return pricingCategoryOpenAI
	}
	return pricingCategoryUnknown
}

func pricingRowMatchesCategoryKind(row PricingRow, kind pricingCategoryKind) bool {
	groupText := strings.ToLower(strings.TrimSpace(row.GroupName + " " + row.GroupDesc))
	switch kind {
	case pricingCategoryClaude:
		return !containsAny(groupText, "codex", "openai", "gpt") ||
			containsAny(groupText, "claude", "claud", "anthropic")
	case pricingCategoryOpenAI:
		return !containsAny(groupText, "claude", "claud", "anthropic") ||
			containsAny(groupText, "codex", "openai", "gpt")
	default:
		return true
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func sub2APIUserPriceRowRaw(row Sub2APIUserPriceRow) []byte {
	data, err := json.Marshal(row)
	if err != nil {
		return []byte(`{}`)
	}
	return data
}

func (s *Server) lowestSnapshotEvent(ctx context.Context, rule Rule, inserted PriceSnapshot, previous PriceSnapshot, previousErr error, expectedCacheHitRatio float64, latencyWeightPerSecond float64) (PriceSnapshot, bool, bool) {
	current, err := s.store.CheapestLatestSnapshot(ctx, rule.Category, inserted.ModelName, expectedCacheHitRatio, latencyWeightPerSecond)
	if err != nil {
		log.Printf("load current cheapest snapshot for rule %d model %q: %v", rule.ID, inserted.ModelName, err)
		return PriceSnapshot{}, false, false
	}
	if current.ID != inserted.ID {
		return current, false, false
	}
	if previousErr != nil {
		if notFound(previousErr) {
			return current, true, false
		}
		log.Printf("load previous cheapest snapshot for rule %d model %q: %v", rule.ID, inserted.ModelName, previousErr)
		return current, false, true
	}
	if sameLowestSnapshot(previous, current) {
		return current, false, true
	}
	if !lowerThanPreviousLowest(previous, current, expectedCacheHitRatio, latencyWeightPerSecond) {
		return current, false, true
	}
	return current, true, false
}

func (s *Server) syncBestAvailableCandidate(ctx context.Context, rule Rule, modelName string, expectedCacheHitRatio float64, latencyWeightPerSecond float64) (bool, bool, error) {
	settings, settingsErr := s.store.GetIntegrationSettings(ctx)
	if settingsErr != nil {
		log.Printf("load integration settings for sync candidates: %v", settingsErr)
	}
	if settingsErr == nil && !settings.Sub2APIEnabled {
		_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, "主站 sub2api 同步开关未开启，已跳过同步", "")
		return false, true, nil
	}
	balanceThreshold := settings.UpstreamBalanceThreshold
	keepCount := normalizeSub2APISyncKeepCount(settings.Sub2APISyncKeepCount)
	refreshedRules := map[int64]bool{}
	var fallbackErrors []string
	var lastCandidates []PriceSnapshot
	var retained []syncCandidateMainResult
	for pass := 0; pass < 2; pass++ {
		candidates, skippedLowBalance, err := s.store.SyncCandidates(ctx, rule.Category, modelName, expectedCacheHitRatio, balanceThreshold, latencyWeightPerSecond, keepCount)
		if err != nil {
			log.Printf("load sync candidates for rule %d model %q: %v", rule.ID, modelName, err)
			return false, false, nil
		}
		lastCandidates = candidates
		if len(candidates) == 0 {
			if len(skippedLowBalance) > 0 {
				if notifySkipped := s.recordLowBalanceSkips(ctx, skippedLowBalance); len(notifySkipped) > 0 {
					s.notifyLowBalanceSkip(ctx, rule, notifySkipped, PriceSnapshot{})
				}
				return false, true, nil
			}
			return false, false, nil
		}
		if pass == 0 && len(skippedLowBalance) > 0 {
			s.updateLowBalanceStatuses(ctx, skippedLowBalance)
			if notifySkipped := s.recordLowBalanceNotifications(ctx, lowBalanceSkippedBelowCandidate(skippedLowBalance, candidates[0], expectedCacheHitRatio, latencyWeightPerSecond)); len(notifySkipped) > 0 {
				s.notifyLowBalanceSkip(ctx, rule, notifySkipped, candidates[0])
			}
		}

		refreshedThisPass := false
		for _, candidate := range candidates {
			candidateCtx, cancelCandidate := context.WithTimeout(context.WithoutCancel(ctx), syncCandidateTimeout)
			candidateLabelText := candidateLabel(candidate)
			s.updateRuleSyncStatus(candidateCtx, rule.ID, fmt.Sprintf("正在尝试同步低价候选：%s", candidateLabelText), "")
			candidateRule, _, _, err := s.store.GetRuleWithSource(candidateCtx, candidate.RuleID)
			if err != nil {
				log.Printf("load candidate rule %d for sync: %v", candidate.RuleID, err)
				reason := fmt.Sprintf("读取候选规则失败：%s", localizeSyncError(err))
				fallbackErrors = append(fallbackErrors, fmt.Sprintf("%s: %s", candidateLabelText, reason))
				s.updateRuleSyncStatus(candidateCtx, rule.ID, fmt.Sprintf("跳过低价候选：%s；原因：%s", candidateLabelText, reason), "")
				cancelCandidate()
				continue
			}
			if reason, ok := s.syncThresholdSkipReason(candidateCtx, candidateRule, candidate); !ok {
				s.updateRuleSyncStatus(candidateCtx, candidateRule.ID, reason, "")
				if candidateRule.ID != rule.ID {
					s.updateRuleSyncStatus(candidateCtx, rule.ID, fmt.Sprintf("跳过低价候选：%s；原因：%s", candidateLabelText, reason), "")
				}
				cancelCandidate()
				continue
			}
			signature := syncCandidateSignature(candidate)
			lastSignature, signatureErr := s.store.RuleSyncSignature(candidateCtx, candidateRule.ID)
			if signatureErr != nil && !notFound(signatureErr) {
				log.Printf("load sync signature for candidate rule %d: %v", candidateRule.ID, signatureErr)
			}
			notifySync := true
			if signature != "" && signature == lastSignature {
				notifySync = false
			}
			result, err := s.syncCandidateSnapshotToMainResult(candidateCtx, candidateRule, candidate, signature, notifySync, false)
			attempted := result.Attempted
			if err == nil {
				retained = appendRetainedSyncResult(retained, result)
				if candidateRule.ID != rule.ID {
					s.updateRuleSyncStatus(candidateCtx, rule.ID, fmt.Sprintf("不是当前可同步最低价，已切换同步：%s", candidateLabelText), "")
				}
				if len(retained) >= keepCount {
					if disableErr := s.disableNonRetainedSyncAccounts(candidateCtx, retained, settings); disableErr != nil {
						cancelCandidate()
						return attempted, true, disableErr
					}
					s.updateRetainedSyncStatus(candidateCtx, rule.ID, retained, keepCount)
					cancelCandidate()
					return attempted, true, nil
				}
				cancelCandidate()
				continue
			}
			if isSub2APISyncDisabledError(err) {
				status := "主站 sub2api 同步开关未开启，已跳过同步"
				s.updateRuleSyncStatus(candidateCtx, candidateRule.ID, status, "")
				if candidateRule.ID != rule.ID {
					s.updateRuleSyncStatus(candidateCtx, rule.ID, fmt.Sprintf("%s：%s", status, candidateLabelText), "")
				}
				cancelCandidate()
				return attempted, true, nil
			}
			if isStaleGroupSyncError(err) {
				status := "检测到上游低价分组已失效，已立即刷新该上游规则并重新参与排名"
				s.updateRuleSyncStatus(candidateCtx, candidateRule.ID, status, "")
				_ = s.store.MarkMissingSnapshotGroupsInvalid(candidateCtx, candidateRule.ID, candidate.ModelName, nil, "同步测试失败：上游低价分组已失效，已触发重新监控")
				if refreshedRules[candidateRule.ID] {
					fallbackErrors = append(fallbackErrors, fmt.Sprintf("%s/%s: %v", candidate.SiteName, candidate.GroupName, err))
					cancelCandidate()
					continue
				}
				refreshedRules[candidateRule.ID] = true
				if _, refreshErr := s.runRuleWithoutSync(candidateCtx, candidateRule.ID); refreshErr != nil {
					fallbackErrors = append(fallbackErrors, fmt.Sprintf("%s/%s: 刷新上游规则失败：%v", candidate.SiteName, candidate.GroupName, refreshErr))
					cancelCandidate()
					continue
				}
				fallbackErrors = append(fallbackErrors, fmt.Sprintf("%s/%s: %v", candidate.SiteName, candidate.GroupName, err))
				refreshedThisPass = true
				cancelCandidate()
				break
			}
			if isFallbackSyncError(err) {
				fallbackErrors = append(fallbackErrors, fmt.Sprintf("%s/%s: %v", candidate.SiteName, candidate.GroupName, err))
				status := fallbackSyncStatus(err)
				s.updateRuleSyncStatus(candidateCtx, candidateRule.ID, status, "")
				if candidateRule.ID != rule.ID {
					s.updateRuleSyncStatus(candidateCtx, rule.ID, fmt.Sprintf("低价候选同步失败，继续尝试下一个：%s；原因：%s", candidateLabelText, strings.TrimPrefix(status, "跳过该低价候选：")), "")
				}
				cancelCandidate()
				continue
			}
			s.recordSyncFailure(candidateCtx, candidateRule, candidate, err)
			cancelCandidate()
			return attempted, true, err
		}
		if !refreshedThisPass || pass > 0 {
			break
		}
	}
	if len(retained) > 0 {
		if disableErr := s.disableNonRetainedSyncAccounts(ctx, retained, settings); disableErr != nil {
			return true, true, disableErr
		}
		s.updateRetainedSyncStatus(ctx, rule.ID, retained, keepCount)
		return true, true, nil
	}
	if len(fallbackErrors) > 0 {
		err := fmt.Errorf("所有可同步低价候选都失败：%s", strings.Join(fallbackErrors, "；"))
		_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, err.Error(), "")
		return true, true, err
	}
	if len(lastCandidates) > 0 {
		_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, fmt.Sprintf("不是当前可同步最低价：%s %s", lastCandidates[0].SiteName, lastCandidates[0].GroupName), "")
	}
	return false, true, nil
}

func (s *Server) updateRuleSyncStatus(ctx context.Context, ruleID int64, status string, errText string) {
	writeCtx := ctx
	var cancel context.CancelFunc
	if writeCtx == nil || writeCtx.Err() != nil {
		writeCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	if err := s.store.UpdateRuleSyncStatus(writeCtx, ruleID, status, errText); err != nil {
		log.Printf("update rule %d sync status: %v", ruleID, err)
	}
}

func appendRetainedSyncResult(results []syncCandidateMainResult, result syncCandidateMainResult) []syncCandidateMainResult {
	if result.Account.ID <= 0 {
		return results
	}
	for _, existing := range results {
		if existing.Account.ID == result.Account.ID {
			return results
		}
	}
	return append(results, result)
}

func (s *Server) disableNonRetainedSyncAccounts(ctx context.Context, retained []syncCandidateMainResult, settings IntegrationSettings) error {
	if len(retained) == 0 {
		return nil
	}
	keepIDs := make([]int64, 0, len(retained))
	var groups []sub2Group
	var platform string
	for _, result := range retained {
		if result.Account.ID > 0 {
			keepIDs = append(keepIDs, result.Account.ID)
		}
		if len(groups) == 0 && len(result.Groups) > 0 {
			groups = result.Groups
		}
		if platform == "" {
			platform = result.Platform
		}
	}
	if len(keepIDs) == 0 || len(groups) == 0 {
		return nil
	}
	sub2, err := s.sub2APIClient(ctx, true)
	if err != nil {
		return err
	}
	if err := sub2.DisableOtherAPIKeyAccountsForGroupsKeeping(ctx, platform, keepIDs, groups, settings.Sub2APISyncAccountMode); err != nil {
		groupNames := make([]string, 0, len(groups))
		for _, group := range groups {
			groupNames = append(groupNames, group.Name)
		}
		return fmt.Errorf("关闭同分组其他主站账号失败：分组 %s，原因：%w", strings.Join(groupNames, ", "), err)
	}
	return nil
}

func (s *Server) updateRetainedSyncStatus(ctx context.Context, ruleID int64, retained []syncCandidateMainResult, keepCount int) {
	if keepCount <= 1 || len(retained) <= 1 {
		return
	}
	labels := make([]string, 0, len(retained))
	for _, result := range retained {
		label := fmt.Sprintf("#%d", result.Account.ID)
		if result.Snapshot.GroupName != "" {
			label += " " + result.Snapshot.GroupName
		}
		labels = append(labels, label)
	}
	s.updateRuleSyncStatus(ctx, ruleID, fmt.Sprintf("同步成功：已保留 %d/%d 个低价主站账号：%s", len(retained), keepCount, strings.Join(labels, "，")), "")
}

func (s *Server) recordLowBalanceSkips(ctx context.Context, skipped []PriceSnapshot) []PriceSnapshot {
	if len(skipped) == 0 {
		return nil
	}
	s.updateLowBalanceStatuses(ctx, skipped)
	return s.recordLowBalanceNotifications(ctx, skipped)
}

func (s *Server) updateLowBalanceStatuses(ctx context.Context, skipped []PriceSnapshot) {
	for _, snapshot := range skipped {
		if snapshot.RuleID <= 0 {
			continue
		}
		_ = s.store.UpdateRuleSyncStatus(ctx, snapshot.RuleID, lowBalanceStatus(snapshot), "")
	}
}

func (s *Server) recordLowBalanceNotifications(ctx context.Context, skipped []PriceSnapshot) []PriceSnapshot {
	if len(skipped) == 0 {
		return nil
	}
	var notifySkipped []PriceSnapshot
	for _, snapshot := range lowBalanceNotifyWindow(skipped) {
		signature := lowBalanceNotificationSignature(snapshot)
		if signature == "" {
			continue
		}
		inserted, err := s.store.RecordLowBalanceNotification(ctx, signature)
		if err != nil {
			log.Printf("record low balance notification for %s: %v", signature, err)
			continue
		}
		if inserted {
			notifySkipped = append(notifySkipped, snapshot)
		}
	}
	return notifySkipped
}

func lowBalanceSkippedBelowCandidate(skipped []PriceSnapshot, candidate PriceSnapshot, expectedCacheHitRatio float64, latencyWeightPerSecond float64) []PriceSnapshot {
	if candidate.ID <= 0 {
		return skipped
	}
	filtered := make([]PriceSnapshot, 0, len(skipped))
	candidateRow := pricingRowFromSnapshot(candidate)
	for _, snapshot := range skipped {
		if pricingRowLessWithComparisonCost(pricingRowFromSnapshot(snapshot), candidateRow, expectedCacheHitRatio, latencyWeightPerSecond) {
			filtered = append(filtered, snapshot)
		}
	}
	return filtered
}

func lowBalanceNotifyWindow(skipped []PriceSnapshot) []PriceSnapshot {
	const notifyLimit = 5
	if len(skipped) <= notifyLimit {
		return skipped
	}
	return skipped[:notifyLimit]
}

func lowBalanceStatus(snapshot PriceSnapshot) string {
	return fmt.Sprintf("跳过余额不足：%s %s %s", snapshot.SiteName, snapshot.GroupName, formatBalance(snapshot.UpstreamBalance, snapshot.BalanceUnit))
}

func fallbackSyncStatus(err error) string {
	text := localizeSyncError(err)
	const maxLen = 240
	if len(text) > maxLen {
		text = text[:maxLen] + "..."
	}
	return "跳过该低价候选：" + text
}

func (s *Server) recordSyncFailure(ctx context.Context, rule Rule, candidate PriceSnapshot, err error) {
	if err == nil {
		return
	}
	if isSub2APISyncDisabledError(err) {
		_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, "主站 sub2api 同步开关未开启，已跳过同步", "")
		return
	}
	failureSignature := syncFailureSignature(candidate, err)
	count, paused, shouldNotify, recordErr := s.store.RecordRuleSyncFailure(ctx, rule.ID, "同步失败", localizeSyncError(err), failureSignature, syncFailurePauseThreshold)
	if recordErr != nil {
		log.Printf("record sync failure for rule %d: %v", rule.ID, recordErr)
		return
	}
	if paused {
		log.Printf("paused rule %d after %d sync failures: %v", rule.ID, count, err)
	}
	if shouldNotify {
		s.notifySyncFailure(ctx, rule, Site{Name: candidate.SiteName, BaseURL: candidate.SiteBaseURL}, pricingRowFromSnapshot(candidate), err)
	}
}

func (s *Server) syncCandidateSnapshotToMain(ctx context.Context, rule Rule, candidate PriceSnapshot, signature string, notifySync bool) (bool, error) {
	result, err := s.syncCandidateSnapshotToMainResult(ctx, rule, candidate, signature, notifySync, true)
	return result.Attempted, err
}

type syncCandidateMainResult struct {
	Attempted bool
	Account   sub2Account
	Groups    []sub2Group
	Platform  string
	Snapshot  PriceSnapshot
}

func (s *Server) syncCandidateSnapshotToMainResult(ctx context.Context, rule Rule, candidate PriceSnapshot, signature string, notifySync bool, disableOthers bool) (syncCandidateMainResult, error) {
	candidateRule, site, upstream, err := s.store.GetRuleWithSource(ctx, candidate.RuleID)
	if err != nil {
		return syncCandidateMainResult{}, err
	}
	row := pricingRowFromSnapshot(candidate)
	keyName := upstreamKeyName(candidateRule, candidate.ModelName)
	switch strings.ToLower(strings.TrimSpace(candidate.SourceType)) {
	case "", RuleSourceNewAPI:
		client, userID, token, err := s.newAPIClientForSite(ctx, site, false)
		if err != nil {
			s.saveNewAPISession(ctx, site, client, site.UserID, site.AccessToken, err.Error())
			return syncCandidateMainResult{}, fmt.Errorf("candidate %s auth NewAPI upstream: %w", candidateLabel(candidate), err)
		}
		apiKey, keyAction, err := createNewAPIUpstreamKey(ctx, client, userID, token, keyName, candidate.GroupName)
		if err != nil && isSessionAuthError(err) {
			client, userID, token, err = s.newAPIClientForSite(ctx, site, true)
			if err == nil {
				apiKey, keyAction, err = createNewAPIUpstreamKey(ctx, client, userID, token, keyName, candidate.GroupName)
			}
		}
		if err != nil {
			s.saveNewAPISession(ctx, site, client, userID, token, err.Error())
			return syncCandidateMainResult{Attempted: true, Snapshot: candidate}, fmt.Errorf("candidate %s create NewAPI key for group %s: %w", candidateLabel(candidate), candidate.GroupName, err)
		}
		s.saveNewAPISession(ctx, site, client, userID, token, "")
		result, err := s.syncUpstreamKeyToMainSub2APIWithOptions(ctx, rule, site.Name, site.BaseURL, apiKey, row, candidate, keyAction, signature, notifySync, disableOthers)
		return syncCandidateMainResult{Attempted: true, Account: result.Account, Groups: result.Groups, Platform: result.Platform, Snapshot: candidate}, err
	case RuleSourceSub2API:
		group, err := sub2GroupFromSnapshot(candidate)
		if err != nil {
			return syncCandidateMainResult{}, fmt.Errorf("candidate %s load sub2api group from snapshot: %w", candidateLabel(candidate), err)
		}
		apiKey, keyAction, err := s.ensureSub2APIUpstreamAPIKey(ctx, upstream, keyName, group)
		if err != nil {
			return syncCandidateMainResult{Attempted: true, Snapshot: candidate}, fmt.Errorf("candidate %s create sub2api key for group %s: %w", candidateLabel(candidate), candidate.GroupName, err)
		}
		result, err := s.syncUpstreamKeyToMainSub2APIWithOptions(ctx, rule, upstream.Name, upstream.BaseURL, apiKey, row, candidate, keyAction, signature, notifySync, disableOthers)
		return syncCandidateMainResult{Attempted: true, Account: result.Account, Groups: result.Groups, Platform: result.Platform, Snapshot: candidate}, err
	default:
		return syncCandidateMainResult{}, fmt.Errorf("unsupported sync candidate source type %q", candidate.SourceType)
	}
}

func pricingRowFromSnapshot(snapshot PriceSnapshot) PricingRow {
	groupRatio := 0.0
	if snapshot.GroupRatio != nil {
		groupRatio = *snapshot.GroupRatio
	}
	return PricingRow{
		ModelName:        snapshot.ModelName,
		GroupName:        snapshot.GroupName,
		GroupDesc:        snapshot.GroupDesc,
		QuotaType:        snapshot.QuotaType,
		GroupRatio:       groupRatio,
		InputPrice:       snapshot.InputPrice,
		OutputPrice:      snapshot.OutputPrice,
		CacheReadPrice:   snapshot.CacheReadPrice,
		CacheWritePrice:  snapshot.CacheWritePrice,
		RequestPrice:     snapshot.RequestPrice,
		RequestLatencyMS: snapshot.RequestLatencyMS,
	}
}

func priceSnapshotFromPricingRow(row PricingRow, sourceName string, sourceBaseURL string) PriceSnapshot {
	return PriceSnapshot{
		SiteName:         sourceName,
		SiteBaseURL:      sourceBaseURL,
		ModelName:        row.ModelName,
		GroupName:        row.GroupName,
		GroupDesc:        row.GroupDesc,
		QuotaType:        row.QuotaType,
		GroupRatio:       ptr(row.GroupRatio),
		InputPrice:       row.InputPrice,
		OutputPrice:      row.OutputPrice,
		CacheReadPrice:   row.CacheReadPrice,
		CacheWritePrice:  row.CacheWritePrice,
		RequestPrice:     row.RequestPrice,
		RequestLatencyMS: row.RequestLatencyMS,
	}
}

func candidateLabel(snapshot PriceSnapshot) string {
	parts := []string{}
	if sourceType := strings.TrimSpace(snapshot.SourceType); sourceType != "" {
		parts = append(parts, sourceType)
	}
	if siteName := strings.TrimSpace(snapshot.SiteName); siteName != "" {
		parts = append(parts, siteName)
	}
	if account := strings.TrimSpace(snapshot.SourceAccount); account != "" {
		parts = append(parts, "账号 "+account)
	}
	if groupName := strings.TrimSpace(snapshot.GroupName); groupName != "" {
		parts = append(parts, groupName)
	}
	if snapshot.GroupRatio != nil {
		parts = append(parts, fmt.Sprintf("倍率 %s", fmtFloatPtr(snapshot.GroupRatio)))
	}
	if len(parts) == 0 {
		return "unknown candidate"
	}
	return strings.Join(parts, " / ")
}

func sourceAccountForSite(site Site) string {
	return strings.TrimSpace(site.Username)
}

func sourceAccountForSub2APIUpstream(upstream Sub2APIUpstream) string {
	if email := strings.TrimSpace(upstream.Email); email != "" {
		return email
	}
	token := strings.TrimSpace(upstream.AuthToken)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("token:%x", sum[:6])
}

func sub2GroupFromSnapshot(snapshot PriceSnapshot) (sub2Group, error) {
	var raw Sub2APIUserPriceRow
	if len(snapshot.Raw) > 0 {
		_ = json.Unmarshal(snapshot.Raw, &raw)
	}
	groupID := raw.GroupID
	if groupID <= 0 {
		return sub2Group{}, fmt.Errorf("sub2api snapshot %d does not include group id", snapshot.ID)
	}
	rate := 0.0
	if snapshot.GroupRatio != nil {
		rate = *snapshot.GroupRatio
	}
	return sub2Group{
		ID:       groupID,
		Name:     firstNonEmpty(raw.GroupName, snapshot.GroupName),
		Platform: firstNonEmpty(raw.GroupPlatform, snapshot.GroupDesc),
		Rate:     rate,
	}, nil
}

func syncCandidateSignature(snapshot PriceSnapshot) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(snapshot.SourceType)),
		strconv.FormatInt(snapshot.SiteID, 10),
		strconv.FormatInt(snapshot.Sub2APIUpstreamID, 10),
		strings.ToLower(strings.TrimSpace(snapshot.SiteBaseURL)),
		strings.ToLower(strings.TrimSpace(snapshot.SourceAccount)),
		strings.ToLower(strings.TrimSpace(snapshot.ModelName)),
		strings.ToLower(strings.TrimSpace(snapshot.GroupName)),
		formatFloatPtr(snapshot.GroupRatio),
		formatFloatPtr(snapshot.InputPrice),
		formatFloatPtr(snapshot.OutputPrice),
		formatFloatPtr(snapshot.CacheReadPrice),
		formatFloatPtr(snapshot.CacheWritePrice),
		formatFloatPtr(snapshot.RequestPrice),
	}
	return strings.Join(parts, "|")
}

func syncFailureSignature(snapshot PriceSnapshot, err error) string {
	parts := []string{
		syncCandidateSignature(snapshot),
	}
	if err != nil {
		parts = append(parts, strings.ToLower(strings.TrimSpace(err.Error())))
	}
	return strings.Join(parts, "|")
}

func lowBalanceNotificationSignature(snapshot PriceSnapshot) string {
	sourceType := strings.ToLower(strings.TrimSpace(snapshot.SourceType))
	if sourceType == "" {
		sourceType = RuleSourceNewAPI
	}
	switch sourceType {
	case RuleSourceSub2API:
		baseURL := strings.ToLower(strings.TrimSpace(snapshot.SiteBaseURL))
		account := strings.ToLower(strings.TrimSpace(snapshot.SourceAccount))
		if baseURL != "" && account != "" {
			return fmt.Sprintf("%s|%s|%s", sourceType, baseURL, account)
		}
		if snapshot.Sub2APIUpstreamID > 0 {
			return fmt.Sprintf("%s|%d", sourceType, snapshot.Sub2APIUpstreamID)
		}
	default:
		if snapshot.SiteID > 0 {
			return fmt.Sprintf("%s|%d", sourceType, snapshot.SiteID)
		}
	}
	baseURL := strings.ToLower(strings.TrimSpace(snapshot.SiteBaseURL))
	if baseURL == "" {
		return ""
	}
	return sourceType + "|" + baseURL
}

func isFallbackSyncError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	needles := []string{
		"permission",
		"forbidden",
		"unauthorized",
		"not allowed",
		"does not support",
		"unsupported",
		"test main sub2api account",
		"test failed",
		"no access",
		"无权",
		"权限",
		"禁止",
		"不支持",
		"不可用",
		"测试失败",
		"连接测试失败",
		"not found",
		"was not found",
		"invalid url",
		"http 429",
		"http 502",
		"api returned 502",
		"error code: 502",
		"bad gateway",
		"api returned 503",
		"service temporarily unavailable",
		"too many requests",
		"rate limit",
		"tls handshake timeout",
		"timeout",
		"eof",
		"connection reset",
		"connection refused",
		"临时限流",
		"请求过于频繁",
		"超时",
		"token key",
		"/api/token/",
	}
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func isSub2APISyncDisabledError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "sub2api sync is disabled") ||
		strings.Contains(text, "主站 sub2api 同步开关未开启")
}

func isStaleGroupSyncError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	needles := []string{
		"no available channel for model",
		"model_not_found",
		"under group",
		"当前分组",
		"所选分组",
		"分组已失效",
		"分组不存在",
		"模型不存在",
		"没有可用渠道支持模型",
	}
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func (s *Server) shouldSyncGlobalCheapestWithBalance(ctx context.Context, rule Rule, snapshot PriceSnapshot) (bool, []PriceSnapshot, bool) {
	settings, err := s.store.GetIntegrationSettings(ctx)
	if err != nil {
		log.Printf("load integration settings for sync candidate: %v", err)
		return false, nil, false
	}
	candidate, skipped, err := s.store.CheapestSyncCandidate(ctx, rule.Category, snapshot.ModelName, settings.ExpectedCacheHitRatio, settings.UpstreamBalanceThreshold, effectiveLatencyWeight(settings), settings.Sub2APISyncKeepCount)
	if err != nil {
		log.Printf("load cheapest sync candidate for rule %d model %q: %v", rule.ID, snapshot.ModelName, err)
		return false, skipped, false
	}
	if reason, ok := s.syncThresholdSkipReason(ctx, rule, candidate); !ok {
		_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, reason, "")
		return false, skipped, true
	}
	if candidate.ID == snapshot.ID {
		return true, skipped, false
	}
	if snapshotBalanceInsufficient(snapshot, settings.UpstreamBalanceThreshold) {
		_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, fmt.Sprintf("跳过余额不足：%s %s %s", snapshot.SiteName, snapshot.GroupName, formatBalance(snapshot.UpstreamBalance, snapshot.BalanceUnit)), "")
		return false, skipped, true
	}
	_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, fmt.Sprintf("不是当前可同步最低价：%s %s", candidate.SiteName, candidate.GroupName), "")
	return false, skipped, true
}

func (s *Server) syncThresholdSkipReason(ctx context.Context, rule Rule, snapshot PriceSnapshot) (string, bool) {
	settings, err := s.store.GetIntegrationSettings(ctx)
	if err != nil {
		return fmt.Sprintf("跳过阈值限制：读取设置失败：%v", err), false
	}
	thresholdRatio := syncThresholdRatioForCategory(settings, rule.Category)
	if thresholdRatio == nil || *thresholdRatio <= 0 {
		return "", true
	}
	ratio := *thresholdRatio
	official, err := officialPriceThreshold(ctx, snapshot.ModelName, ratio)
	if err != nil {
		return fmt.Sprintf("跳过阈值限制：%s", localizeSyncError(err)), false
	}
	expectedCacheHitRatio := settings.ExpectedCacheHitRatio
	actualCost := pricingRowExpectedPrice(pricingRowFromSnapshot(snapshot), expectedCacheHitRatio)
	officialCost := pricingRowExpectedPrice(official, expectedCacheHitRatio)
	if actualCost > officialCost {
		return fmt.Sprintf("跳过阈值限制：分类 %s，预期成本 %s > %s（预期缓存命中率 %.2f）", normalizeCategorySlug(rule.Category), fmtFloat(actualCost), fmtFloat(officialCost), expectedCacheHitRatio), false
	}
	return "", true
}

func officialPriceThreshold(ctx context.Context, modelName string, ratio float64) (PricingRow, error) {
	officialPrices, _, err := loadOfficialPrices(ctx, defaultOfficialPriceURL)
	if err != nil {
		return PricingRow{}, err
	}
	entry := asMap(officialPrices[strings.TrimSpace(modelName)])
	if len(entry) == 0 {
		return PricingRow{}, fmt.Errorf("official price not found for model %q", modelName)
	}
	return PricingRow{
		ModelName:      modelName,
		GroupRatio:     ratio,
		InputPrice:     multiplyPerMillionPtr(officialPrice(entry, "input_cost_per_token"), ratio),
		OutputPrice:    multiplyPerMillionPtr(officialPrice(entry, "output_cost_per_token"), ratio),
		CacheReadPrice: multiplyPerMillionPtr(officialPrice(entry, "cache_read_input_token_cost"), ratio),
		CacheWritePrice: firstFloatPtr(
			multiplyPerMillionPtr(officialPrice(entry, "cache_creation_input_token_cost"), ratio),
			multiplyPerMillionPtr(officialPrice(entry, "cache_creation_input_token_cost_above_1hr"), ratio),
		),
		RequestPrice: multiplyPerMillionPtr(officialPrice(entry, "input_cost_per_request"), ratio),
	}, nil
}

func overPrice(actual *float64, threshold *float64) bool {
	if actual == nil || threshold == nil {
		return false
	}
	const priceEpsilon = 1e-9
	return *actual-*threshold > priceEpsilon
}

func syncThresholdStatus(category string, label string, actual *float64, threshold *float64, ratio float64) string {
	return fmt.Sprintf("跳过阈值限制：分类 %s，阈值倍率 %.9g，%s价格 %s > %s", normalizeCategorySlug(category), ratio, chinesePriceLabel(label), fmtFloatPtr(actual), fmtFloatPtr(threshold))
}

func chinesePriceLabel(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "input":
		return "输入"
	case "output":
		return "输出"
	case "cache read":
		return "缓存读"
	case "cache write":
		return "缓存写"
	case "request":
		return "请求"
	default:
		if strings.TrimSpace(label) == "" {
			return "模型"
		}
		return label
	}
}

func firstFloatPtr(values ...*float64) *float64 {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func createNewAPIUpstreamKey(ctx context.Context, newAPI *NewAPIClient, userID int64, token string, keyName string, groupName string) (string, string, error) {
	apiKey, action, err := newAPI.EnsureAPIKeyForGroup(ctx, userID, token, keyName, groupName)
	if err != nil {
		return "", "", err
	}
	return apiKey, action, nil
}

func (s *Server) ensureSub2APIUpstreamAPIKey(ctx context.Context, upstream Sub2APIUpstream, keyName string, group sub2Group) (string, string, error) {
	cfg := sub2APIUserSourceConfig{
		UpstreamID: upstream.ID,
		BaseURL:    upstream.BaseURL,
		AuthToken:  upstream.AuthToken,
		Email:      upstream.Email,
		Password:   upstream.Password,
		TOTPCode:   upstream.TOTPCode,
		CookieJar:  upstream.CookieJar,
	}
	client, err := s.sub2APIClientForUserSource(ctx, cfg, false)
	if err != nil {
		return "", "", err
	}
	key, action, err := client.EnsureAPIKeyForGroup(ctx, keyName, group)
	if err != nil && isSessionAuthError(err) {
		client, err = s.sub2APIClientForUserSource(ctx, cfg, true)
		if err == nil {
			key, action, err = client.EnsureAPIKeyForGroup(ctx, keyName, group)
		}
	}
	if err != nil {
		s.saveSub2APIUserSession(ctx, cfg, client, err.Error())
		return "", "", err
	}
	if strings.TrimSpace(key.Key) == "" {
		err := fmt.Errorf("sub2api api key %q did not return key value", keyName)
		s.saveSub2APIUserSession(ctx, cfg, client, err.Error())
		return "", "", err
	}
	s.saveSub2APIUserSession(ctx, cfg, client, "")
	return key.Key, action, nil
}

func upstreamKeyName(rule Rule, modelName string) string {
	return fmt.Sprintf("pm-r%d-%s-%s", rule.ID, sanitizeTokenName(rule.ModelKeyword), sanitizeTokenName(modelName))
}

func (s *Server) syncUpstreamKeyToMainSub2API(ctx context.Context, rule Rule, sourceName string, sourceBaseURL string, apiKey string, row PricingRow, keyAction string) error {
	return s.syncUpstreamKeyToMainSub2APIWithSignature(ctx, rule, sourceName, sourceBaseURL, apiKey, row, PriceSnapshot{}, keyAction, "", true)
}

func (s *Server) syncUpstreamKeyToMainSub2APIWithSignature(ctx context.Context, rule Rule, sourceName string, sourceBaseURL string, apiKey string, row PricingRow, snapshot PriceSnapshot, keyAction string, signature string, notifySync bool) error {
	_, err := s.syncUpstreamKeyToMainSub2APIWithOptions(ctx, rule, sourceName, sourceBaseURL, apiKey, row, snapshot, keyAction, signature, notifySync, true)
	return err
}

type syncToMainResult struct {
	Account  sub2Account
	Groups   []sub2Group
	Platform string
}

func (s *Server) syncUpstreamKeyToMainSub2APIWithOptions(ctx context.Context, rule Rule, sourceName string, sourceBaseURL string, apiKey string, row PricingRow, snapshot PriceSnapshot, keyAction string, signature string, notifySync bool, disableOthers bool) (syncToMainResult, error) {
	settings, err := s.store.GetIntegrationSettings(ctx)
	if err != nil {
		return syncToMainResult{}, fmt.Errorf("load integration settings: %w", err)
	}
	if !settings.Sub2APIEnabled {
		return syncToMainResult{}, fmt.Errorf("sub2api sync is disabled")
	}
	sub2, err := s.sub2APIClient(ctx, true)
	if err != nil {
		return syncToMainResult{}, err
	}

	category, err := s.store.GetCategoryBySlug(ctx, rule.Category)
	if err != nil && !notFound(err) {
		return syncToMainResult{}, fmt.Errorf("load category sync target: %w", err)
	}
	targets := categorySyncTargets(rule, category)
	groups := make([]sub2Group, 0, len(targets))
	groupRate := ptr(row.GroupRatio)
	for _, target := range targets {
		group, err := sub2.EnsureGroupByIDOrNameWithRate(ctx, target.ID, target.Name, groupRate)
		if err != nil {
			return syncToMainResult{}, err
		}
		groups = append(groups, group)
	}
	groupNames := make([]string, 0, len(groups))
	for _, group := range groups {
		groupNames = append(groupNames, group.Name)
	}
	platform := syncPlatformForRule(rule, category)
	accountName := fmt.Sprintf("%s %s %s", sourceName, strings.Join(groupNames, "+"), row.GroupName)
	account, action, alreadyMatchedGroups, err := sub2.UpsertAPIKeyAccountGroupsWithRateAndMode(ctx, platform, accountName, sourceBaseURL, apiKey, groups, nil, settings.Sub2APISyncAccountMode)
	if err != nil {
		return syncToMainResult{}, err
	}
	if err := sub2.PrioritizeOpenAIAPIKeyAccountForGroupsWithRate(ctx, account.ID, groups, nil); err != nil {
		return syncToMainResult{}, err
	}
	if !alreadyMatchedGroups {
		if err := sub2.TestAccountConnection(ctx, account.ID, row.ModelName); err != nil {
			return syncToMainResult{}, fmt.Errorf("主站账号连接测试失败：账号 #%d，模型 %s，主站分组 %s，上游低价分组 %s，原因：%w",
				account.ID, row.ModelName, strings.Join(groupNames, ", "), row.GroupName, err)
		}
	}
	if disableOthers {
		if err := sub2.DisableOtherAPIKeyAccountsForGroups(ctx, platform, account.ID, groups, settings.Sub2APISyncAccountMode); err != nil {
			return syncToMainResult{}, fmt.Errorf("关闭同分组其他主站账号失败：分组 %s，原因：%w", strings.Join(groupNames, ", "), err)
		}
	}
	if snapshot.ID == 0 {
		snapshot = priceSnapshotFromPricingRow(row, sourceName, sourceBaseURL)
	}
	if notifySync {
		s.notifySyncUpdate(ctx, rule, Site{Name: sourceName, BaseURL: sourceBaseURL}, snapshot, action, account)
	}
	status := fmt.Sprintf("同步成功：主站账号%s，上游key%s，低价分组 %s，倍率 %s，同步到主站分组 %s",
		chineseSyncAction(action), chineseSyncAction(keyAction), row.GroupName, fmtFloat(row.GroupRatio), strings.Join(groupNames, ", "))
	if !alreadyMatchedGroups {
		status += fmt.Sprintf("，已测试模型 %s", row.ModelName)
	} else {
		status += "，已跳过真实测试"
	}
	if !notifySync {
		status = "复核成功：" + strings.TrimPrefix(status, "同步成功：")
	}
	if strings.TrimSpace(signature) != "" {
		return syncToMainResult{Account: account, Groups: groups, Platform: platform}, s.store.UpdateRuleSyncSuccess(ctx, rule.ID, status, signature)
	}
	return syncToMainResult{Account: account, Groups: groups, Platform: platform}, s.store.UpdateRuleSyncStatus(ctx, rule.ID, status, "")
}

func chineseSyncAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "created":
		return "已创建"
	case "updated":
		return "已更新"
	case "reused":
		return "已复用"
	default:
		if strings.TrimSpace(action) == "" {
			return "已处理"
		}
		return action
	}
}

func localizeSyncError(err error) string {
	if err == nil {
		return ""
	}
	return localizeErrorText(err.Error())
}

func syncPlatformForRule(rule Rule, category Category) string {
	value := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		rule.Category,
		category.Slug,
		category.Name,
		rule.ModelKeyword,
		rule.ModelName,
	}, " ")))
	if strings.Contains(value, "claude") || strings.Contains(value, "claud") || strings.Contains(value, "anthropic") {
		return sub2PlatformAnthropic
	}
	return sub2PlatformOpenAI
}

func categorySyncTargets(rule Rule, category Category) []Sub2APIGroupRef {
	if category.ID > 0 && len(category.Sub2APIMainGroups) > 0 {
		return category.Sub2APIMainGroups
	}
	target := Sub2APIGroupRef{ID: rule.Sub2APIGroupID, Name: strings.TrimSpace(rule.Sub2APIGroupName)}
	if category.ID > 0 {
		if category.Sub2APIMainGroupID > 0 {
			target.ID = category.Sub2APIMainGroupID
		}
		if strings.TrimSpace(category.Sub2APIMainGroupName) != "" {
			target.Name = category.Sub2APIMainGroupName
		}
	}
	if target.Name == "" {
		target.Name = firstNonEmpty(rule.CategoryName, rule.Category)
	}
	return []Sub2APIGroupRef{target}
}

func fmtFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func fmtFloatPtr(value *float64) string {
	if value == nil {
		return "-"
	}
	return fmtFloat(*value)
}

func (s *Server) startScheduler(ctx context.Context, fallbackInterval time.Duration) {
	if fallbackInterval <= 0 {
		fallbackInterval = time.Minute
	}
	if !s.runEnabledRulesIfIdle(ctx) {
		log.Printf("scheduler skipped because a polling round is already running")
	}
	for {
		roundMinutes, _ := s.schedulerIntervals(ctx, fallbackInterval)
		roundInterval := time.Duration(roundMinutes) * time.Minute
		if roundMinutes <= 0 {
			roundInterval = fallbackInterval
		}
		if !sleepContext(ctx, roundInterval) {
			return
		}
		if !s.runEnabledRulesIfIdle(ctx) {
			log.Printf("scheduler skipped because a polling round is already running")
			continue
		}
	}
}

func (s *Server) runEnabledRules(ctx context.Context) {
	if invalidated, err := s.store.MarkCategoryMismatchedSnapshotsInvalid(ctx, "snapshot group does not match rule category"); err != nil {
		log.Printf("mark category mismatched snapshots invalid: %v", err)
	} else if invalidated > 0 {
		log.Printf("marked %d category mismatched snapshots invalid", invalidated)
	}
	if deleted, err := s.store.PruneExpiredInvalidSnapshots(ctx, 7*24*time.Hour); err != nil {
		log.Printf("prune expired invalid snapshots: %v", err)
	} else if deleted > 0 {
		log.Printf("pruned %d expired invalid snapshots", deleted)
	}
	roundInterval, ruleDelay := s.schedulerIntervals(ctx, s.cfg.MonitorInterval)
	ids, err := s.store.ScheduledRuleIDs(ctx, 500)
	if err != nil {
		log.Printf("scheduler list rules: %v", err)
		return
	}
	if len(ids) > 0 {
		log.Printf("scheduler found %d scheduled rule(s)", len(ids))
	}
	rules := make([]scheduledRuleSource, 0, len(ids))
	for _, id := range ids {
		loadCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		rule, site, upstream, ruleErr := s.store.GetRuleWithSource(loadCtx, id)
		cancel()
		if ruleErr != nil {
			log.Printf("scheduler load rule %d: %v", id, ruleErr)
			continue
		}
		if !rule.Enabled || !rule.ScheduleEnabled {
			continue
		}
		rules = append(rules, scheduledRuleSource{rule: rule, site: site, upstream: upstream})
	}
	groups := groupScheduledRulesBySource(rules)
	if len(groups) > 0 {
		log.Printf("scheduler grouped %d scheduled rule(s) into %d source site batch(es)", len(rules), len(groups))
	}
	for index, group := range groups {
		if index > 0 {
			_, ruleDelay = s.schedulerIntervals(ctx, s.cfg.MonitorInterval)
			if !sleepContext(ctx, ruleDelay) {
				return
			}
		}
		log.Printf("scheduler running source site batch %s with %d rule(s)", group.label, len(group.rules))
		batchCtx := withRuleBatchSessionCache(ctx)
		for _, item := range group.rules {
			runCtx, cancel := context.WithTimeout(batchCtx, 60*time.Second)
			runStartedAt := time.Now()
			_, err := s.runRuleWithSource(runCtx, item.rule, item.site, item.upstream)
			if markErr := s.store.MarkRuleScheduled(context.Background(), item.rule.ID, runStartedAt, roundInterval); markErr != nil {
				log.Printf("scheduler mark rule %d scheduled: %v", item.rule.ID, markErr)
			} else {
				log.Printf("scheduler rule %d scheduled next run in %d minute(s)", item.rule.ID, roundInterval)
			}
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("scheduler rule %d: %v", item.rule.ID, err)
			}
		}
	}
}

type scheduledRuleSource struct {
	rule     Rule
	site     Site
	upstream Sub2APIUpstream
}

type scheduledRuleSourceBatch struct {
	key   string
	label string
	rules []scheduledRuleSource
}

func groupScheduledRulesBySource(rules []scheduledRuleSource) []scheduledRuleSourceBatch {
	if len(rules) == 0 {
		return nil
	}
	groups := make([]scheduledRuleSourceBatch, 0)
	indexByKey := make(map[string]int, len(rules))
	for _, item := range rules {
		key, label := scheduledRuleSourceKey(item)
		if idx, ok := indexByKey[key]; ok {
			groups[idx].rules = append(groups[idx].rules, item)
			continue
		}
		indexByKey[key] = len(groups)
		groups = append(groups, scheduledRuleSourceBatch{
			key:   key,
			label: label,
			rules: []scheduledRuleSource{item},
		})
	}
	return groups
}

func scheduledRuleSourceKey(item scheduledRuleSource) (string, string) {
	sourceType := strings.ToLower(strings.TrimSpace(item.rule.SourceType))
	switch sourceType {
	case "", RuleSourceNewAPI:
		id := firstPositiveInt64(item.site.ID, item.rule.SiteID)
		label := fmt.Sprintf("newapi:%d", id)
		if name := strings.TrimSpace(firstNonEmpty(item.site.Name, item.rule.SiteName, item.rule.SourceName)); name != "" {
			label += ":" + name
		}
		return fmt.Sprintf("newapi:%d", id), label
	case RuleSourceSub2API:
		id := firstPositiveInt64(item.upstream.ID, item.rule.Sub2APIUpstreamID)
		label := fmt.Sprintf("sub2api:%d", id)
		if name := strings.TrimSpace(firstNonEmpty(item.upstream.Name, item.rule.Sub2APIUpstreamName, item.rule.SourceName)); name != "" {
			label += ":" + name
		}
		return fmt.Sprintf("sub2api:%d", id), label
	default:
		return fmt.Sprintf("%s:%d", sourceType, item.rule.ID), fmt.Sprintf("%s:%d", sourceType, item.rule.ID)
	}
}

func (s *Server) schedulerIntervals(ctx context.Context, fallbackInterval time.Duration) (int, time.Duration) {
	settings, err := s.store.GetIntegrationSettings(ctx)
	if err != nil {
		log.Printf("load scheduler settings: %v", err)
		return durationMinutes(fallbackInterval), time.Minute
	}
	roundMinutes, ruleDelaySeconds := normalizeMonitorScheduleSettings(settings.MonitorIntervalMinutes, settings.MonitorRuleDelaySeconds)
	return roundMinutes, time.Duration(ruleDelaySeconds) * time.Second
}

func durationMinutes(value time.Duration) int {
	if value <= 0 {
		return 15
	}
	minutes := int(value / time.Minute)
	if minutes <= 0 {
		minutes = 1
	}
	return minutes
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	if duration <= 0 {
		return true
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func decodeRequest(r *http.Request, out any) error {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/json") {
		return json.NewDecoder(r.Body).Decode(out)
	}
	if err := r.ParseForm(); err != nil {
		return err
	}
	data, _ := json.Marshal(formMap(r))
	return json.Unmarshal(data, out)
}

func formMap(r *http.Request) map[string]any {
	out := map[string]any{}
	for key := range r.Form {
		value := r.Form.Get(key)
		if key == "site_id" || key == "interval_minutes" || key == "sub2api_group_id" || key == "sub2api_upstream_id" || key == "group_id" || key == "sub2api_main_group_id" {
			if id, err := strconv.ParseInt(value, 10, 64); err == nil {
				out[key] = id
				continue
			}
		}
		if key == "enabled" || key == "schedule_enabled" || key == "sync_enabled" || key == "sub2api_enabled" {
			out[key] = value == "true" || value == "on" || value == "1"
			continue
		}
		out[key] = value
	}
	return out
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return strings.Repeat("*", len(value))
	}
	return value[:4] + strings.Repeat("*", 8) + value[len(value)-4:]
}

func redactSub2Accounts(accounts []sub2Account) []sub2Account {
	out := make([]sub2Account, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, redactSub2Account(account))
	}
	return out
}

func redactSub2APIUpstreams(upstreams []Sub2APIUpstream) []Sub2APIUpstream {
	out := make([]Sub2APIUpstream, 0, len(upstreams))
	for _, upstream := range upstreams {
		out = append(out, redactSub2APIUpstream(upstream))
	}
	return out
}

type sub2APIUpstreamView struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	BaseURL     string     `json:"base_url"`
	Email       string     `json:"email"`
	Password    string     `json:"password,omitempty"`
	AuthToken   string     `json:"auth_token,omitempty"`
	TOTPCode    string     `json:"totp_code,omitempty"`
	LastError   string     `json:"last_error"`
	LastCheckAt *time.Time `json:"last_check_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func sub2APIUpstreamViews(upstreams []Sub2APIUpstream) []sub2APIUpstreamView {
	out := make([]sub2APIUpstreamView, 0, len(upstreams))
	for _, upstream := range upstreams {
		out = append(out, sub2APIUpstreamView{
			ID:          upstream.ID,
			Name:        upstream.Name,
			BaseURL:     upstream.BaseURL,
			Email:       upstream.Email,
			Password:    upstream.Password,
			AuthToken:   upstream.AuthToken,
			TOTPCode:    upstream.TOTPCode,
			LastError:   upstream.LastError,
			LastCheckAt: upstream.LastCheckAt,
			CreatedAt:   upstream.CreatedAt,
			UpdatedAt:   upstream.UpdatedAt,
		})
	}
	return out
}

func redactSub2APIUpstream(upstream Sub2APIUpstream) Sub2APIUpstream {
	upstream.Password = ""
	upstream.TOTPCode = ""
	upstream.AuthToken = maskSecret(upstream.AuthToken)
	return upstream
}

func redactSub2Account(account sub2Account) sub2Account {
	if account.Credentials == nil {
		return account
	}
	redacted := make(map[string]any, len(account.Credentials))
	for key, value := range account.Credentials {
		if strings.Contains(strings.ToLower(key), "key") || strings.Contains(strings.ToLower(key), "token") {
			if text, ok := value.(string); ok {
				redacted[key] = maskSecret(text)
				continue
			}
		}
		redacted[key] = value
	}
	account.Credentials = redacted
	return account
}

func sanitizeTokenName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			out.WriteRune(r)
		case r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == '-' || r == '_':
			out.WriteRune(r)
		default:
			out.WriteByte('-')
		}
		if out.Len() >= 16 {
			break
		}
	}
	cleaned := strings.Trim(out.String(), "-")
	if cleaned == "" {
		return "rule"
	}
	return cleaned
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	message = localizeErrorText(message)
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    http.StatusText(status),
			"message": message,
		},
	})
}

var indexTemplate = template.Must(template.New("index").Delims("[[", "]]").Parse(indexHTML))
