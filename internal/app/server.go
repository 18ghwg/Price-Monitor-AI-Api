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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Server struct {
	cfg    Config
	db     *pgxpool.Pool
	store  Store
	secret []byte
}

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
		log.Printf("price monitor scheduler enabled, scan interval %s", cfg.MonitorInterval)
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
	mux.HandleFunc("POST /api/sub2api/accounts/upsert", s.upsertSub2APIAccount)
	mux.HandleFunc("POST /api/sub2api/accounts/{id}/enable", s.enableSub2APIAccount)
	mux.HandleFunc("POST /api/sub2api/accounts/{id}/disable", s.disableSub2APIAccount)
	mux.HandleFunc("POST /api/sub2api/accounts/{id}/apikey", s.updateSub2APIAccountKey)
	mux.HandleFunc("GET /api/sites", s.listSites)
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
			w.Header().Set("WWW-Authenticate", `Basic realm="newapi-price-monitor"`)
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
		ID         int64      `json:"id"`
		SourceType string     `json:"source_type"`
		Name       string     `json:"name"`
		BaseURL    string     `json:"base_url"`
		Username   string     `json:"username"`
		Email      string     `json:"email,omitempty"`
		LastError  string     `json:"last_error"`
		LastRunAt  *time.Time `json:"last_run_at"`
	}
	out := make([]siteDTO, 0, len(sites)+len(upstreams))
	for _, site := range sites {
		out = append(out, siteDTO{
			ID: site.ID, SourceType: RuleSourceNewAPI, Name: site.Name, BaseURL: site.BaseURL, Username: site.Username,
			LastError: site.LastError, LastRunAt: site.LastRunAt,
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
	_, inspect, err := s.verifySub2APIUpstreamInput(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	saved, err := s.store.CreateSub2APIUpstream(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	_ = s.store.UpdateSub2APIUpstreamCheck(r.Context(), saved.ID, time.Now(), "")
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
	_, inspect, err := s.verifySub2APIUpstreamInput(r.Context(), verifyInput)
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
	_ = s.store.UpdateSub2APIUpstreamCheck(r.Context(), id, time.Now(), "")
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
	inspect, err := s.inspectSub2APIUser(ctx, sub2APIUserPriceInput{
		BaseURL:   upstream.BaseURL,
		Email:     upstream.Email,
		Password:  upstream.Password,
		AuthToken: upstream.AuthToken,
		TOTPCode:  upstream.TOTPCode,
	})
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

func (s *Server) verifyNewAPISiteInput(ctx context.Context, input SiteInput) (int64, string, error) {
	input = normalizeSiteInput(input)
	if input.Name == "" || input.BaseURL == "" || input.Username == "" || input.Password == "" {
		return 0, "", fmt.Errorf("site name, base url, username and password are required")
	}
	client, err := NewNewAPIClient(input.BaseURL)
	if err != nil {
		return 0, "", err
	}
	userID, err := client.Login(ctx, input.Username, input.Password, input.TOTPCode)
	if err != nil {
		return 0, "", fmt.Errorf("login NewAPI upstream: %w", err)
	}
	token, err := client.GenerateSystemAccessToken(ctx, userID)
	if err != nil {
		return 0, "", fmt.Errorf("generate NewAPI system access token: %w", err)
	}
	return userID, token, nil
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
	userID, token, err := s.verifyNewAPISiteInput(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	site, err := s.store.CreateSite(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	runAt := time.Now()
	if err := s.store.UpdateSiteRun(r.Context(), site.ID, userID, token, runAt, ""); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	site.UserID = userID
	site.AccessToken = token
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
		input.TOTPCode != ""
	var userID int64
	var token string
	if credentialsChanged {
		verifyInput := input
		if strings.TrimSpace(verifyInput.Password) == "" {
			verifyInput.Password = existing.Password
		}
		userID, token, err = s.verifyNewAPISiteInput(r.Context(), verifyInput)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
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
		if err := s.store.UpdateSiteRun(r.Context(), site.ID, userID, token, runAt, ""); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		site.UserID = userID
		site.AccessToken = token
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
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"count":     len(snapshots),
		"snapshots": snapshots,
	}})
}

func (s *Server) listSnapshots(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	snapshots, err := s.store.LatestSnapshots(r.Context(), limit, r.URL.Query().Get("category"))
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
	if !rule.Enabled {
		return nil, fmt.Errorf("rule is disabled")
	}
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
	client, err := NewNewAPIClient(site.BaseURL)
	if err != nil {
		return nil, err
	}
	userID, err := client.Login(ctx, site.Username, site.Password, site.TOTPCode)
	if err != nil {
		_ = s.store.UpdateSiteRun(ctx, site.ID, site.UserID, site.AccessToken, time.Now(), err.Error())
		return nil, err
	}
	token := site.AccessToken
	if token == "" || userID != site.UserID {
		token, err = client.GenerateSystemAccessToken(ctx, userID)
		if err != nil {
			_ = s.store.UpdateSiteRun(ctx, site.ID, userID, "", time.Now(), err.Error())
			return nil, err
		}
	}

	pricing, _, err := client.FetchPricing(ctx, userID, token)
	if err != nil {
		token, tokenErr := client.GenerateSystemAccessToken(ctx, userID)
		if tokenErr == nil {
			pricing, _, err = client.FetchPricing(ctx, userID, token)
		}
	}
	if err != nil {
		_ = s.store.UpdateSiteRun(ctx, site.ID, userID, token, time.Now(), err.Error())
		return nil, err
	}
	balance, balanceErr := client.FetchBalance(ctx, userID, token)
	if balanceErr != nil {
		log.Printf("fetch newapi balance for site %d: %v", site.ID, balanceErr)
		balance.Unit = "usd"
	}

	rows, err := BuildCheapestKeywordRows(pricing, rule.ModelKeyword)
	if err != nil {
		_ = s.store.UpdateSiteRun(ctx, site.ID, userID, token, time.Now(), err.Error())
		return nil, err
	}
	if len(rows) == 0 {
		err := fmt.Errorf("no pricing rows found for model %q", rule.ModelKeyword)
		_ = s.store.MarkMissingSnapshotGroupsInvalid(ctx, rule.ID, rule.ModelKeyword, nil, "model or upstream group disappeared or is no longer cheapest")
		_ = s.store.UpdateSiteRun(ctx, site.ID, userID, token, time.Now(), err.Error())
		return nil, err
	}

	snapshots := make([]PriceSnapshot, 0, len(rows))
	activeGroups := make([]string, 0, len(rows))
	var syncErr error
	syncAttempted := false
	syncDecisionRecorded := false
	for _, row := range rows {
		activeGroups = append(activeGroups, row.GroupName)
		previousLowest, previousLowestErr := s.store.CheapestLatestSnapshot(ctx, rule.Category, row.ModelName)
		snapshot := PriceSnapshot{
			RuleID:          rule.ID,
			SourceType:      RuleSourceNewAPI,
			SiteID:          site.ID,
			SiteName:        site.Name,
			SiteBaseURL:     site.BaseURL,
			Category:        rule.Category,
			CategoryName:    rule.CategoryName,
			ModelKeyword:    rule.ModelKeyword,
			ModelName:       row.ModelName,
			GroupName:       row.GroupName,
			GroupDesc:       row.GroupDesc,
			QuotaType:       row.QuotaType,
			GroupRatio:      ptr(row.GroupRatio),
			InputPrice:      row.InputPrice,
			OutputPrice:     row.OutputPrice,
			CacheReadPrice:  row.CacheReadPrice,
			CacheWritePrice: row.CacheWritePrice,
			RequestPrice:    row.RequestPrice,
			UpstreamBalance: balance.Value,
			BalanceUnit:     balance.Unit,
			Raw:             PricingRowRaw(row),
		}
		snapshot, err = s.store.InsertSnapshot(ctx, snapshot)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
		currentLowest, newLowest, stableLowest := s.lowestSnapshotEvent(ctx, rule, snapshot, previousLowest, previousLowestErr)
		if stableLowest {
			syncDecisionRecorded = true
		}
		if newLowest {
			s.notifyPriceChange(ctx, previousLowest, currentLowest, lowestSnapshotChanges(previousLowest, currentLowest))
		}
		if rule.SyncEnabled {
			attempted, decisionRecorded, err := s.syncBestAvailableCandidate(ctx, rule, row.ModelName)
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
	_ = s.store.UpdateSiteRun(ctx, site.ID, userID, token, time.Now(), "")
	if rule.SyncEnabled && !syncAttempted && syncErr == nil && !syncDecisionRecorded {
		_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, "not current cheapest", "")
	}
	return snapshots, nil
}

func (s *Server) runSub2APIRule(ctx context.Context, rule Rule, upstream Sub2APIUpstream) ([]PriceSnapshot, error) {
	if upstream.ID <= 0 {
		return nil, fmt.Errorf("sub2api source site is required")
	}
	client, groups, userRates, err := s.fetchSub2APIUserClientGroups(ctx, sub2APIUserPriceInput{
		Sub2APIUpstreamID: upstream.ID,
	})
	if err != nil {
		return nil, err
	}
	balance, balanceErr := client.FetchBalance(ctx)
	if balanceErr != nil {
		log.Printf("fetch sub2api balance for upstream %d: %v", upstream.ID, balanceErr)
		balance.Unit = "usd"
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
	rows := cheapestSub2PriceRows(result.Rows)
	if len(rows) == 0 {
		_ = s.store.MarkMissingSnapshotGroupsInvalid(ctx, rule.ID, rule.ModelKeyword, nil, "model or upstream group disappeared or is no longer cheapest")
		return nil, fmt.Errorf("no sub2api pricing rows found for model %q", rule.ModelKeyword)
	}

	snapshots := make([]PriceSnapshot, 0, len(rows))
	activeGroups := make([]string, 0, len(rows))
	var syncErr error
	syncAttempted := false
	syncDecisionRecorded := false
	for _, row := range rows {
		activeGroups = append(activeGroups, row.GroupName)
		previousLowest, previousLowestErr := s.store.CheapestLatestSnapshot(ctx, rule.Category, row.ModelName)
		cacheWritePrice := row.FinalCacheWritePerMillion
		if cacheWritePrice == nil {
			cacheWritePrice = row.FinalCacheWrite1hPerMillion
		}
		snapshot := PriceSnapshot{
			RuleID:            rule.ID,
			SourceType:        RuleSourceSub2API,
			Sub2APIUpstreamID: upstream.ID,
			SiteName:          upstream.Name,
			SiteBaseURL:       upstream.BaseURL,
			Category:          rule.Category,
			CategoryName:      rule.CategoryName,
			ModelKeyword:      rule.ModelKeyword,
			ModelName:         row.ModelName,
			GroupName:         row.GroupName,
			GroupDesc:         strings.TrimSpace(row.GroupPlatform),
			QuotaType:         0,
			GroupRatio:        ptr(row.EffectiveRate),
			InputPrice:        row.FinalInputPerMillion,
			OutputPrice:       row.FinalOutputPerMillion,
			CacheReadPrice:    row.FinalCacheReadPerMillion,
			CacheWritePrice:   cacheWritePrice,
			UpstreamBalance:   balance.Value,
			BalanceUnit:       balance.Unit,
			Raw:               sub2APIUserPriceRowRaw(row),
		}
		snapshot, err = s.store.InsertSnapshot(ctx, snapshot)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
		currentLowest, newLowest, stableLowest := s.lowestSnapshotEvent(ctx, rule, snapshot, previousLowest, previousLowestErr)
		if stableLowest {
			syncDecisionRecorded = true
		}
		if newLowest {
			s.notifyPriceChange(ctx, previousLowest, currentLowest, lowestSnapshotChanges(previousLowest, currentLowest))
		}
		if rule.SyncEnabled {
			attempted, decisionRecorded, err := s.syncBestAvailableCandidate(ctx, rule, row.ModelName)
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
		_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, "not current cheapest", "")
	}
	return snapshots, nil
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

func cheapestSub2PriceRows(rows []Sub2APIUserPriceRow) []Sub2APIUserPriceRow {
	cheapest := map[string]Sub2APIUserPriceRow{}
	for _, row := range rows {
		if strings.TrimSpace(row.ModelName) == "" {
			continue
		}
		current, ok := cheapest[row.ModelName]
		if !ok || sub2APIUserPriceRowLess(row, current) {
			cheapest[row.ModelName] = row
		}
	}
	models := make([]string, 0, len(cheapest))
	for model := range cheapest {
		models = append(models, model)
	}
	sort.Strings(models)
	out := make([]Sub2APIUserPriceRow, 0, len(models))
	for _, model := range models {
		out = append(out, cheapest[model])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if sub2APIUserPriceRowLess(out[i], out[j]) {
			return true
		}
		if sub2APIUserPriceRowLess(out[j], out[i]) {
			return false
		}
		return out[i].ModelName < out[j].ModelName
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

func sub2APIUserPriceRowRaw(row Sub2APIUserPriceRow) []byte {
	data, err := json.Marshal(row)
	if err != nil {
		return []byte(`{}`)
	}
	return data
}

func (s *Server) lowestSnapshotEvent(ctx context.Context, rule Rule, inserted PriceSnapshot, previous PriceSnapshot, previousErr error) (PriceSnapshot, bool, bool) {
	current, err := s.store.CheapestLatestSnapshot(ctx, rule.Category, inserted.ModelName)
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
	return current, true, false
}

func (s *Server) syncBestAvailableCandidate(ctx context.Context, rule Rule, modelName string) (bool, bool, error) {
	candidates, skippedLowBalance, err := s.store.SyncCandidates(ctx, rule.Category, modelName)
	if err != nil {
		log.Printf("load sync candidates for rule %d model %q: %v", rule.ID, modelName, err)
		return false, false, nil
	}
	if len(candidates) == 0 {
		if len(skippedLowBalance) > 0 {
			s.notifyLowBalanceSkip(ctx, rule, skippedLowBalance, PriceSnapshot{})
			_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, "skip low balance: no available candidate", "")
			return false, true, nil
		}
		return false, false, nil
	}
	if len(skippedLowBalance) > 0 {
		s.notifyLowBalanceSkip(ctx, rule, skippedLowBalance, candidates[0])
	}

	lastSignature, signatureErr := s.store.RuleSyncSignature(ctx, rule.ID)
	if signatureErr != nil && !notFound(signatureErr) {
		log.Printf("load sync signature for rule %d: %v", rule.ID, signatureErr)
	}

	var fallbackErrors []string
	for _, candidate := range candidates {
		if reason, ok := s.syncThresholdSkipReason(ctx, rule, candidate); !ok {
			_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, reason, "")
			return false, true, nil
		}
		signature := syncCandidateSignature(candidate)
		if signature != "" && signature == lastSignature {
			return false, true, nil
		}
		attempted, err := s.syncCandidateSnapshotToMain(ctx, rule, candidate, signature)
		if err == nil {
			return attempted, true, nil
		}
		if isFallbackSyncError(err) {
			fallbackErrors = append(fallbackErrors, fmt.Sprintf("%s/%s: %v", candidate.SiteName, candidate.GroupName, err))
			_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, fmt.Sprintf("fallback from %s %s", candidate.SiteName, candidate.GroupName), err.Error())
			continue
		}
		_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, "error", err.Error())
		s.notifySyncFailure(ctx, rule, Site{Name: candidate.SiteName, BaseURL: candidate.SiteBaseURL}, pricingRowFromSnapshot(candidate), err)
		return attempted, true, err
	}
	if len(fallbackErrors) > 0 {
		err := fmt.Errorf("all sync candidates failed: %s", strings.Join(fallbackErrors, "；"))
		_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, "error", err.Error())
		return true, true, err
	}
	if len(candidates) > 0 {
		_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, fmt.Sprintf("not current available cheapest: %s %s", candidates[0].SiteName, candidates[0].GroupName), "")
	}
	return false, true, nil
}

func (s *Server) syncCandidateSnapshotToMain(ctx context.Context, rule Rule, candidate PriceSnapshot, signature string) (bool, error) {
	candidateRule, site, upstream, err := s.store.GetRuleWithSource(ctx, candidate.RuleID)
	if err != nil {
		return false, err
	}
	row := pricingRowFromSnapshot(candidate)
	keyName := upstreamKeyName(candidateRule, candidate.ModelName)
	switch strings.ToLower(strings.TrimSpace(candidate.SourceType)) {
	case "", RuleSourceNewAPI:
		client, err := NewNewAPIClient(site.BaseURL)
		if err != nil {
			return false, err
		}
		userID, err := client.Login(ctx, site.Username, site.Password, site.TOTPCode)
		if err != nil {
			return false, err
		}
		token := site.AccessToken
		if token == "" || userID != site.UserID {
			token, err = client.GenerateSystemAccessToken(ctx, userID)
			if err != nil {
				return false, err
			}
		}
		apiKey, keyAction, err := createNewAPIUpstreamKey(ctx, client, userID, token, keyName, candidate.GroupName)
		if err != nil {
			return true, err
		}
		return true, s.syncUpstreamKeyToMainSub2APIWithSignature(ctx, rule, site.Name, site.BaseURL, apiKey, row, keyAction, signature)
	case RuleSourceSub2API:
		group, err := sub2GroupFromSnapshot(candidate)
		if err != nil {
			return false, err
		}
		apiKey, keyAction, err := s.ensureSub2APIUpstreamAPIKey(ctx, upstream, keyName, group)
		if err != nil {
			return true, err
		}
		return true, s.syncUpstreamKeyToMainSub2APIWithSignature(ctx, rule, upstream.Name, upstream.BaseURL, apiKey, row, keyAction, signature)
	default:
		return false, fmt.Errorf("unsupported sync candidate source type %q", candidate.SourceType)
	}
}

func pricingRowFromSnapshot(snapshot PriceSnapshot) PricingRow {
	groupRatio := 0.0
	if snapshot.GroupRatio != nil {
		groupRatio = *snapshot.GroupRatio
	}
	return PricingRow{
		ModelName:       snapshot.ModelName,
		GroupName:       snapshot.GroupName,
		GroupDesc:       snapshot.GroupDesc,
		QuotaType:       snapshot.QuotaType,
		GroupRatio:      groupRatio,
		InputPrice:      snapshot.InputPrice,
		OutputPrice:     snapshot.OutputPrice,
		CacheReadPrice:  snapshot.CacheReadPrice,
		CacheWritePrice: snapshot.CacheWritePrice,
		RequestPrice:    snapshot.RequestPrice,
	}
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
		"no access",
		"无权",
		"权限",
		"禁止",
		"不可用",
		"not found",
		"was not found",
	}
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func (s *Server) shouldSyncGlobalCheapestWithBalance(ctx context.Context, rule Rule, snapshot PriceSnapshot) (bool, []PriceSnapshot, bool) {
	candidate, skipped, err := s.store.CheapestSyncCandidate(ctx, rule.Category, snapshot.ModelName)
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
	if snapshotBalanceInsufficient(snapshot) {
		_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, fmt.Sprintf("skip low balance: %s %s %s", snapshot.SiteName, snapshot.GroupName, formatBalance(snapshot.UpstreamBalance, snapshot.BalanceUnit)), "")
		return false, skipped, true
	}
	_ = s.store.UpdateRuleSyncStatus(ctx, rule.ID, fmt.Sprintf("not current available cheapest: %s %s", candidate.SiteName, candidate.GroupName), "")
	return false, skipped, true
}

func (s *Server) syncThresholdSkipReason(ctx context.Context, rule Rule, snapshot PriceSnapshot) (string, bool) {
	settings, err := s.store.GetIntegrationSettings(ctx)
	if err != nil {
		return fmt.Sprintf("skip threshold: load settings: %v", err), false
	}
	if settings.SyncThresholdRatio == nil || *settings.SyncThresholdRatio <= 0 {
		return "", true
	}
	ratio := *settings.SyncThresholdRatio
	official, err := officialPriceThreshold(ctx, snapshot.ModelName, ratio)
	if err != nil {
		return fmt.Sprintf("skip threshold: %v", err), false
	}
	if overPrice(snapshot.InputPrice, official.InputPrice) {
		return syncThresholdStatus("input", snapshot.InputPrice, official.InputPrice, ratio), false
	}
	if overPrice(snapshot.OutputPrice, official.OutputPrice) {
		return syncThresholdStatus("output", snapshot.OutputPrice, official.OutputPrice, ratio), false
	}
	if overPrice(snapshot.CacheReadPrice, official.CacheReadPrice) {
		return syncThresholdStatus("cache read", snapshot.CacheReadPrice, official.CacheReadPrice, ratio), false
	}
	if overPrice(snapshot.CacheWritePrice, official.CacheWritePrice) {
		return syncThresholdStatus("cache write", snapshot.CacheWritePrice, official.CacheWritePrice, ratio), false
	}
	if overPrice(snapshot.RequestPrice, official.RequestPrice) {
		return syncThresholdStatus("request", snapshot.RequestPrice, official.RequestPrice, ratio), false
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
	return *actual > *threshold
}

func syncThresholdStatus(label string, actual *float64, threshold *float64, ratio float64) string {
	return fmt.Sprintf("skip threshold %.9g: %s %s > %s", ratio, label, fmtFloatPtr(actual), fmtFloatPtr(threshold))
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
	client, err := NewSub2APIClient(upstream.BaseURL, upstream.AuthToken)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(upstream.AuthToken) == "" {
		if err := client.LoginWith2FA(ctx, upstream.Email, upstream.Password, upstream.TOTPCode, ""); err != nil {
			return "", "", err
		}
	}
	key, action, err := client.EnsureAPIKeyForGroup(ctx, keyName, group)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(key.Key) == "" {
		return "", "", fmt.Errorf("sub2api api key %q did not return key value", keyName)
	}
	return key.Key, action, nil
}

func upstreamKeyName(rule Rule, modelName string) string {
	return fmt.Sprintf("pm-r%d-%s-%s", rule.ID, sanitizeTokenName(rule.ModelKeyword), sanitizeTokenName(modelName))
}

func (s *Server) syncUpstreamKeyToMainSub2API(ctx context.Context, rule Rule, sourceName string, sourceBaseURL string, apiKey string, row PricingRow, keyAction string) error {
	return s.syncUpstreamKeyToMainSub2APIWithSignature(ctx, rule, sourceName, sourceBaseURL, apiKey, row, keyAction, "")
}

func (s *Server) syncUpstreamKeyToMainSub2APIWithSignature(ctx context.Context, rule Rule, sourceName string, sourceBaseURL string, apiKey string, row PricingRow, keyAction string, signature string) error {
	settings, err := s.store.GetIntegrationSettings(ctx)
	if err != nil {
		return fmt.Errorf("load integration settings: %w", err)
	}
	if !settings.Sub2APIEnabled {
		return fmt.Errorf("sub2api sync is disabled")
	}
	sub2, err := s.sub2APIClient(ctx, true)
	if err != nil {
		return err
	}

	category, err := s.store.GetCategoryBySlug(ctx, rule.Category)
	if err != nil && !notFound(err) {
		return fmt.Errorf("load category sync target: %w", err)
	}
	targets := categorySyncTargets(rule, category)
	groups := make([]sub2Group, 0, len(targets))
	for _, target := range targets {
		group, err := sub2.EnsureGroupByIDOrName(ctx, target.ID, target.Name)
		if err != nil {
			return err
		}
		groups = append(groups, group)
	}
	groupNames := make([]string, 0, len(groups))
	for _, group := range groups {
		groupNames = append(groupNames, group.Name)
	}
	platform := syncPlatformForRule(rule, category)
	accountName := fmt.Sprintf("%s %s %s", sourceName, strings.Join(groupNames, "+"), row.GroupName)
	accountRate := ptr(row.GroupRatio)
	account, action, err := sub2.UpsertAPIKeyAccountGroupsWithRate(ctx, platform, accountName, sourceBaseURL, apiKey, groups, accountRate)
	if err != nil {
		return err
	}
	if err := sub2.PrioritizeOpenAIAPIKeyAccountForGroupsWithRate(ctx, account.ID, groups, accountRate); err != nil {
		return err
	}
	s.notifySyncUpdate(ctx, rule, Site{Name: sourceName, BaseURL: sourceBaseURL}, row, action, account)
	status := action + " " + keyAction + " " + row.GroupName + " rate " + fmtFloat(row.GroupRatio) + " -> " + strings.Join(groupNames, ", ")
	if strings.TrimSpace(signature) != "" {
		return s.store.UpdateRuleSyncSuccess(ctx, rule.ID, status, signature)
	}
	return s.store.UpdateRuleSyncStatus(ctx, rule.ID, status, "")
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

func (s *Server) startScheduler(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.runEnabledRules(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runEnabledRules(ctx)
		}
	}
}

func (s *Server) runEnabledRules(ctx context.Context) {
	if deleted, err := s.store.PruneExpiredInvalidSnapshots(ctx, 7*24*time.Hour); err != nil {
		log.Printf("prune expired invalid snapshots: %v", err)
	} else if deleted > 0 {
		log.Printf("pruned %d expired invalid snapshots", deleted)
	}
	now := time.Now()
	ids, err := s.store.DueRuleIDs(ctx, now, 50)
	if err != nil {
		log.Printf("scheduler list rules: %v", err)
		return
	}
	if len(ids) > 0 {
		log.Printf("scheduler found %d due rule(s)", len(ids))
	}
	for _, id := range ids {
		runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		rule, _, _, ruleErr := s.store.GetRuleWithSource(runCtx, id)
		if ruleErr != nil {
			cancel()
			log.Printf("scheduler load rule %d: %v", id, ruleErr)
			continue
		}
		runStartedAt := time.Now()
		_, err := s.RunRule(runCtx, id)
		if markErr := s.store.MarkRuleScheduled(context.Background(), id, runStartedAt, rule.IntervalMinutes); markErr != nil {
			log.Printf("scheduler mark rule %d scheduled: %v", id, markErr)
		} else {
			log.Printf("scheduler rule %d scheduled next run in %d minute(s)", id, rule.IntervalMinutes)
		}
		cancel()
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("scheduler rule %d: %v", id, err)
		}
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
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    http.StatusText(status),
			"message": message,
		},
	})
}

var indexTemplate = template.Must(template.New("index").Parse(indexHTML))
