package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

const (
	RuleSourceNewAPI  = "newapi"
	RuleSourceSub2API = "sub2api"
)

type SiteInput struct {
	Name        string `json:"name"`
	BaseURL     string `json:"base_url"`
	Username    string `json:"username"`
	UserID      int64  `json:"user_id"`
	Password    string `json:"password"`
	AccessToken string `json:"access_token"`
	TOTPCode    string `json:"totp_code"`
}

type CategoryInput struct {
	Name                 string            `json:"name"`
	Slug                 string            `json:"slug"`
	Sub2APIMainGroupID   int64             `json:"sub2api_main_group_id"`
	Sub2APIMainGroupName string            `json:"sub2api_main_group_name"`
	Sub2APIMainGroups    []Sub2APIGroupRef `json:"sub2api_main_groups"`
	BlockedGroupKeywords []string          `json:"blocked_group_keywords"`
}

type Sub2APIUpstreamInput struct {
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	AuthToken string `json:"auth_token"`
	TOTPCode  string `json:"totp_code"`
}

type RuleInput struct {
	SourceType         string  `json:"source_type"`
	SiteID             int64   `json:"site_id"`
	Sub2APIUpstreamID  int64   `json:"sub2api_upstream_id"`
	Category           string  `json:"category"`
	ModelKeyword       string  `json:"model_keyword"`
	ModelName          string  `json:"model_name"`
	GroupName          string  `json:"group_name"`
	Enabled            bool    `json:"enabled"`
	ScheduleEnabled    bool    `json:"schedule_enabled"`
	IntervalMinutes    int     `json:"interval_minutes"`
	SyncEnabled        bool    `json:"sync_enabled"`
	SyncBaseGroup      string  `json:"sync_base_group"`
	Sub2APIGroupName   string  `json:"sub2api_group_name"`
	Sub2APIGroupID     int64   `json:"sub2api_group_id"`
	SyncThresholdRatio float64 `json:"sync_threshold_ratio"`
	InitialNextRunAt   *time.Time
}

type BulkRuleUpdateInput struct {
	RuleIDs               []int64 `json:"rule_ids"`
	UpdateModelKeyword    bool    `json:"update_model_keyword"`
	ModelKeyword          string  `json:"model_keyword"`
	UpdateIntervalMinutes bool    `json:"update_interval_minutes"`
	IntervalMinutes       int     `json:"interval_minutes"`
	UpdateScheduleEnabled bool    `json:"update_schedule_enabled"`
	ScheduleEnabled       bool    `json:"schedule_enabled"`
	UpdateSyncEnabled     bool    `json:"update_sync_enabled"`
	SyncEnabled           bool    `json:"sync_enabled"`
}

type SettingsInput struct {
	Sub2APIEnabled           bool                           `json:"sub2api_enabled"`
	Sub2APIMainBaseURL       string                         `json:"sub2api_main_base_url"`
	Sub2APIAdminKey          string                         `json:"sub2api_admin_key"`
	Sub2APIBaseURL           string                         `json:"sub2api_base_url"`
	Sub2APIAccessToken       string                         `json:"sub2api_access_token"`
	Sub2APIEmail             string                         `json:"sub2api_email"`
	Sub2APIPassword          string                         `json:"sub2api_password"`
	Sub2APISyncAccountMode   string                         `json:"sub2api_sync_account_mode"`
	MonitorIntervalMinutes   int                            `json:"monitor_interval_minutes"`
	MonitorRuleDelaySeconds  int                            `json:"monitor_rule_delay_seconds"`
	ExpectedCacheHitRatio    float64                        `json:"expected_cache_hit_ratio"`
	UpstreamBalanceThreshold float64                        `json:"upstream_balance_threshold"`
	SyncThresholdRatio       float64                        `json:"sync_threshold_ratio"`
	SyncThresholdRatios      map[string]float64             `json:"sync_threshold_ratios"`
	EmailNotifyEnabled       bool                           `json:"email_notify_enabled"`
	EmailNotifyPriceChange   bool                           `json:"email_notify_price_change"`
	EmailNotifySyncUpdate    bool                           `json:"email_notify_sync_update"`
	SMTPHost                 string                         `json:"smtp_host"`
	SMTPPort                 int                            `json:"smtp_port"`
	SMTPEncryption           string                         `json:"smtp_encryption"`
	SMTPUsername             string                         `json:"smtp_username"`
	SMTPPassword             string                         `json:"smtp_password"`
	SMTPFrom                 string                         `json:"smtp_from"`
	SMTPTo                   string                         `json:"smtp_to"`
	EmailTemplateEnabled     bool                           `json:"email_template_enabled"`
	EmailTemplateSubject     string                         `json:"email_template_subject"`
	EmailTemplateBody        string                         `json:"email_template_body"`
	EmailTemplateConfigs     map[string]EmailTemplateConfig `json:"email_template_configs"`
}

type AdminCredentialInput struct {
	Username        string `json:"username"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s Store) CreateSite(ctx context.Context, input SiteInput) (Site, error) {
	input = normalizeSiteInput(input)
	if input.Name == "" || input.BaseURL == "" || (input.AccessToken == "" && (input.Username == "" || input.Password == "")) {
		return Site{}, fmt.Errorf("site name, base url, system access token or username and password are required")
	}
	if err := s.ensureUniqueSite(ctx, input, 0); err != nil {
		return Site{}, err
	}

	var site Site
	err := s.db.QueryRow(ctx, `
		INSERT INTO sites (name, base_url, username, password, access_token, totp_code)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, base_url, username, password, totp_code, user_id, access_token, cookie_jar, last_error, last_run_at, created_at, updated_at
	`, input.Name, input.BaseURL, input.Username, input.Password, input.AccessToken, input.TOTPCode).Scan(
		&site.ID, &site.Name, &site.BaseURL, &site.Username, &site.Password, &site.TOTPCode,
		&site.UserID, &site.AccessToken, &site.CookieJar, &site.LastError, &site.LastRunAt, &site.CreatedAt, &site.UpdatedAt,
	)
	return site, err
}

func (s Store) ensureUniqueSite(ctx context.Context, input SiteInput, excludeID int64) error {
	if strings.TrimSpace(input.Username) == "" {
		return nil
	}
	var existingID int64
	err := s.db.QueryRow(ctx, `
		SELECT id
		FROM sites
		WHERE base_url = $1
		  AND lower(username) = lower($2)
		  AND ($3::bigint = 0 OR id <> $3)
		ORDER BY id
		LIMIT 1
	`, input.BaseURL, input.Username, excludeID).Scan(&existingID)
	if err == nil {
		return fmt.Errorf("NewAPI 上游站点账号已存在：同一站点地址和用户名不能重复添加")
	}
	if err == pgx.ErrNoRows {
		return nil
	}
	return err
}

func (s Store) ListSites(ctx context.Context) ([]Site, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, base_url, username, password, totp_code, user_id, access_token, cookie_jar, last_error, last_run_at, created_at, updated_at
		FROM sites
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sites []Site
	for rows.Next() {
		var site Site
		if err := rows.Scan(
			&site.ID, &site.Name, &site.BaseURL, &site.Username, &site.Password, &site.TOTPCode,
			&site.UserID, &site.AccessToken, &site.CookieJar, &site.LastError, &site.LastRunAt, &site.CreatedAt, &site.UpdatedAt,
		); err != nil {
			return nil, err
		}
		sites = append(sites, site)
	}
	return sites, rows.Err()
}

func (s Store) GetSite(ctx context.Context, siteID int64) (Site, error) {
	var site Site
	err := s.db.QueryRow(ctx, `
		SELECT id, name, base_url, username, password, totp_code, user_id, access_token, cookie_jar, last_error, last_run_at, created_at, updated_at
		FROM sites
		WHERE id = $1
	`, siteID).Scan(
		&site.ID, &site.Name, &site.BaseURL, &site.Username, &site.Password, &site.TOTPCode,
		&site.UserID, &site.AccessToken, &site.CookieJar, &site.LastError, &site.LastRunAt, &site.CreatedAt, &site.UpdatedAt,
	)
	return site, err
}

func (s Store) UpdateSite(ctx context.Context, siteID int64, input SiteInput) (Site, error) {
	input = normalizeSiteInput(input)
	if siteID <= 0 || input.Name == "" || input.BaseURL == "" || (input.AccessToken == "" && input.Username == "") {
		return Site{}, fmt.Errorf("site id, name, base url and username or system access token are required")
	}
	if err := s.ensureUniqueSite(ctx, input, siteID); err != nil {
		return Site{}, err
	}

	var site Site
	err := s.db.QueryRow(ctx, `
		UPDATE sites
		SET name = $2,
		    base_url = $3,
		    username = $4,
		    password = CASE WHEN $5 = '' THEN password ELSE $5 END,
		    totp_code = $6,
		    access_token = CASE WHEN $7 <> '' THEN $7 WHEN base_url <> $3 OR username <> $4 OR ($5 <> '' AND password <> $5) THEN '' ELSE access_token END,
		    cookie_jar = CASE WHEN $7 <> '' THEN '' WHEN base_url <> $3 OR username <> $4 OR ($5 <> '' AND password <> $5) THEN '' ELSE cookie_jar END,
		    last_error = '',
		    updated_at = now()
		WHERE id = $1
		RETURNING id, name, base_url, username, password, totp_code, user_id, access_token, cookie_jar, last_error, last_run_at, created_at, updated_at
	`, siteID, input.Name, input.BaseURL, input.Username, input.Password, input.TOTPCode, input.AccessToken).Scan(
		&site.ID, &site.Name, &site.BaseURL, &site.Username, &site.Password, &site.TOTPCode,
		&site.UserID, &site.AccessToken, &site.CookieJar, &site.LastError, &site.LastRunAt, &site.CreatedAt, &site.UpdatedAt,
	)
	if err != nil {
		return Site{}, err
	}
	return site, nil
}

func (s Store) DeleteSite(ctx context.Context, siteID int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM sites WHERE id = $1`, siteID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s Store) UpdateSiteRun(ctx context.Context, siteID int64, userID int64, token string, runAt time.Time, lastErr string) error {
	return s.UpdateSiteRunWithCookies(ctx, siteID, userID, token, "", runAt, lastErr)
}

func (s Store) UpdateSiteRunWithCookies(ctx context.Context, siteID int64, userID int64, token string, cookieJar string, runAt time.Time, lastErr string) error {
	lastErr = localizeErrorText(lastErr)
	_, err := s.db.Exec(ctx, `
		UPDATE sites
		SET user_id = $2,
		    access_token = $3,
		    cookie_jar = CASE WHEN $4 <> '' THEN $4 ELSE cookie_jar END,
		    last_run_at = $5,
		    last_error = $6,
		    updated_at = now()
		WHERE id = $1
	`, siteID, userID, token, cookieJar, runAt, lastErr)
	return err
}

func (s Store) CreateSub2APIUpstream(ctx context.Context, input Sub2APIUpstreamInput) (Sub2APIUpstream, error) {
	input = normalizeSub2APIUpstreamInput(input)
	if input.Name == "" || input.BaseURL == "" || (input.AuthToken == "" && (input.Email == "" || input.Password == "")) {
		return Sub2APIUpstream{}, fmt.Errorf("upstream name, base url and login credentials are required")
	}
	if err := s.ensureUniqueSub2APIUpstream(ctx, input, 0); err != nil {
		return Sub2APIUpstream{}, err
	}

	var upstream Sub2APIUpstream
	err := s.db.QueryRow(ctx, `
		INSERT INTO sub2api_upstreams (name, base_url, email, password, auth_token, totp_code)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, base_url, email, password, auth_token, totp_code, cookie_jar, last_error, last_check_at, created_at, updated_at
	`, input.Name, input.BaseURL, input.Email, input.Password, input.AuthToken, input.TOTPCode).Scan(
		&upstream.ID, &upstream.Name, &upstream.BaseURL, &upstream.Email, &upstream.Password, &upstream.AuthToken,
		&upstream.TOTPCode, &upstream.CookieJar, &upstream.LastError, &upstream.LastCheckAt, &upstream.CreatedAt, &upstream.UpdatedAt,
	)
	return upstream, err
}

func (s Store) ListSub2APIUpstreams(ctx context.Context) ([]Sub2APIUpstream, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, base_url, email, password, auth_token, totp_code, cookie_jar, last_error, last_check_at, created_at, updated_at
		FROM sub2api_upstreams
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var upstreams []Sub2APIUpstream
	for rows.Next() {
		var upstream Sub2APIUpstream
		if err := rows.Scan(
			&upstream.ID, &upstream.Name, &upstream.BaseURL, &upstream.Email, &upstream.Password, &upstream.AuthToken,
			&upstream.TOTPCode, &upstream.CookieJar, &upstream.LastError, &upstream.LastCheckAt, &upstream.CreatedAt, &upstream.UpdatedAt,
		); err != nil {
			return nil, err
		}
		upstreams = append(upstreams, upstream)
	}
	return upstreams, rows.Err()
}

func (s Store) GetSub2APIUpstream(ctx context.Context, upstreamID int64) (Sub2APIUpstream, error) {
	var upstream Sub2APIUpstream
	err := s.db.QueryRow(ctx, `
		SELECT id, name, base_url, email, password, auth_token, totp_code, cookie_jar, last_error, last_check_at, created_at, updated_at
		FROM sub2api_upstreams
		WHERE id = $1
	`, upstreamID).Scan(
		&upstream.ID, &upstream.Name, &upstream.BaseURL, &upstream.Email, &upstream.Password, &upstream.AuthToken,
		&upstream.TOTPCode, &upstream.CookieJar, &upstream.LastError, &upstream.LastCheckAt, &upstream.CreatedAt, &upstream.UpdatedAt,
	)
	return upstream, err
}

func (s Store) UpdateSub2APIUpstream(ctx context.Context, upstreamID int64, input Sub2APIUpstreamInput) (Sub2APIUpstream, error) {
	input = normalizeSub2APIUpstreamInput(input)
	if upstreamID <= 0 || input.Name == "" || input.BaseURL == "" {
		return Sub2APIUpstream{}, fmt.Errorf("upstream id, name and base url are required")
	}
	if err := s.ensureUniqueSub2APIUpstream(ctx, input, upstreamID); err != nil {
		return Sub2APIUpstream{}, err
	}

	var upstream Sub2APIUpstream
	err := s.db.QueryRow(ctx, `
		UPDATE sub2api_upstreams
		SET name = $2,
		    base_url = $3,
		    email = $4,
		    password = CASE WHEN $5 = '' THEN password ELSE $5 END,
		    auth_token = CASE WHEN $6 = '' THEN auth_token ELSE $6 END,
		    totp_code = $7,
		    cookie_jar = CASE WHEN base_url <> $3 OR email <> $4 OR ($5 <> '' AND password <> $5) OR ($6 <> '' AND auth_token <> $6) THEN '' ELSE cookie_jar END,
		    last_error = '',
		    updated_at = now()
		WHERE id = $1
		RETURNING id, name, base_url, email, password, auth_token, totp_code, cookie_jar, last_error, last_check_at, created_at, updated_at
	`, upstreamID, input.Name, input.BaseURL, input.Email, input.Password, input.AuthToken, input.TOTPCode).Scan(
		&upstream.ID, &upstream.Name, &upstream.BaseURL, &upstream.Email, &upstream.Password, &upstream.AuthToken,
		&upstream.TOTPCode, &upstream.CookieJar, &upstream.LastError, &upstream.LastCheckAt, &upstream.CreatedAt, &upstream.UpdatedAt,
	)
	return upstream, err
}

func (s Store) ensureUniqueSub2APIUpstream(ctx context.Context, input Sub2APIUpstreamInput, excludeID int64) error {
	if excludeID > 0 && input.Email == "" && input.AuthToken == "" {
		if err := s.db.QueryRow(ctx, `
			SELECT auth_token
			FROM sub2api_upstreams
			WHERE id = $1
		`, excludeID).Scan(&input.AuthToken); err != nil {
			return err
		}
		input.AuthToken = strings.TrimSpace(input.AuthToken)
	}
	if input.Email == "" && input.AuthToken == "" {
		return nil
	}

	var existingID int64
	err := s.db.QueryRow(ctx, `
		SELECT id
		FROM sub2api_upstreams
		WHERE base_url = $1
		  AND (
		    ($2 <> '' AND lower(email) = lower($2))
		    OR ($3 <> '' AND auth_token = $3)
		  )
		  AND ($4::bigint = 0 OR id <> $4)
		ORDER BY id
		LIMIT 1
	`, input.BaseURL, input.Email, input.AuthToken, excludeID).Scan(&existingID)
	if err == nil {
		return fmt.Errorf("sub2api 上游站点账号已存在：同一站点地址和账号不能重复添加")
	}
	if err == pgx.ErrNoRows {
		return nil
	}
	return err
}

func (s Store) DeleteSub2APIUpstream(ctx context.Context, upstreamID int64) error {
	var used int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM monitor_rules WHERE sub2api_upstream_id = $1`, upstreamID).Scan(&used); err != nil {
		return err
	}
	if used > 0 {
		return fmt.Errorf("sub2api upstream is used by %d monitor rule(s)", used)
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM sub2api_upstreams WHERE id = $1`, upstreamID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s Store) UpdateSub2APIUpstreamCheck(ctx context.Context, upstreamID int64, checkedAt time.Time, lastErr string) error {
	return s.UpdateSub2APIUpstreamCheckWithSession(ctx, upstreamID, checkedAt, lastErr, "", "")
}

func (s Store) UpdateSub2APIUpstreamCheckWithSession(ctx context.Context, upstreamID int64, checkedAt time.Time, lastErr string, cookieJar string, authToken string) error {
	lastErr = localizeErrorText(lastErr)
	_, err := s.db.Exec(ctx, `
		UPDATE sub2api_upstreams
		SET last_check_at = $2,
		    last_error = $3,
		    cookie_jar = CASE WHEN $4 <> '' THEN $4 ELSE cookie_jar END,
		    auth_token = CASE WHEN $5 <> '' THEN $5 ELSE auth_token END,
		    updated_at = now()
		WHERE id = $1
	`, upstreamID, checkedAt, lastErr, cookieJar, authToken)
	return err
}

func (s Store) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, slug, sub2api_main_group_id, sub2api_main_group_name, COALESCE(sub2api_main_groups, '[]'::jsonb), COALESCE(blocked_group_keywords, '[]'::jsonb), created_at, updated_at
		FROM categories
		ORDER BY CASE slug WHEN 'codex' THEN 0 WHEN 'claud' THEN 1 WHEN 'other' THEN 2 ELSE 3 END, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []Category
	for rows.Next() {
		var category Category
		var groupsRaw []byte
		var blockedRaw []byte
		if err := rows.Scan(
			&category.ID, &category.Name, &category.Slug, &category.Sub2APIMainGroupID, &category.Sub2APIMainGroupName,
			&groupsRaw, &blockedRaw,
			&category.CreatedAt, &category.UpdatedAt,
		); err != nil {
			return nil, err
		}
		category.Sub2APIMainGroups = normalizeCategoryGroupRefs(category.Sub2APIMainGroupID, category.Sub2APIMainGroupName, groupsRaw)
		category.BlockedGroupKeywords = normalizeStringListFromJSON(blockedRaw)
		categories = append(categories, category)
	}
	return categories, rows.Err()
}

func (s Store) CreateCategory(ctx context.Context, input CategoryInput) (Category, error) {
	name := strings.TrimSpace(input.Name)
	slug := normalizeCategorySlug(input.Slug)
	groups, groupsJSON, err := normalizeCategoryInputGroups(input)
	if err != nil {
		return Category{}, err
	}
	blockedKeywords, blockedJSON := normalizeStringList(input.BlockedGroupKeywords)
	primary := primaryCategoryGroup(groups)
	if slug == "" {
		slug = normalizeCategorySlug(name)
	}
	if name == "" || slug == "" {
		return Category{}, fmt.Errorf("category name is required")
	}

	var category Category
	err = s.db.QueryRow(ctx, `
		INSERT INTO categories (name, slug, sub2api_main_group_id, sub2api_main_group_name, sub2api_main_groups, blocked_group_keywords)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb)
		ON CONFLICT (slug) DO UPDATE
		SET name = EXCLUDED.name,
		    sub2api_main_group_id = EXCLUDED.sub2api_main_group_id,
		    sub2api_main_group_name = EXCLUDED.sub2api_main_group_name,
		    sub2api_main_groups = EXCLUDED.sub2api_main_groups,
		    blocked_group_keywords = EXCLUDED.blocked_group_keywords,
		    updated_at = now()
		RETURNING id, name, slug, sub2api_main_group_id, sub2api_main_group_name, COALESCE(sub2api_main_groups, '[]'::jsonb), COALESCE(blocked_group_keywords, '[]'::jsonb), created_at, updated_at
	`, name, slug, primary.ID, primary.Name, string(groupsJSON), string(blockedJSON)).Scan(
		&category.ID, &category.Name, &category.Slug, &category.Sub2APIMainGroupID, &category.Sub2APIMainGroupName,
		&groupsJSON, &blockedJSON,
		&category.CreatedAt, &category.UpdatedAt,
	)
	category.Sub2APIMainGroups = normalizeCategoryGroupRefs(category.Sub2APIMainGroupID, category.Sub2APIMainGroupName, groupsJSON)
	category.BlockedGroupKeywords = blockedKeywords
	return category, err
}

func (s Store) UpdateCategory(ctx context.Context, categoryID int64, input CategoryInput) (Category, error) {
	name := strings.TrimSpace(input.Name)
	slug := normalizeCategorySlug(input.Slug)
	groups, groupsJSON, err := normalizeCategoryInputGroups(input)
	if err != nil {
		return Category{}, err
	}
	blockedKeywords, blockedJSON := normalizeStringList(input.BlockedGroupKeywords)
	primary := primaryCategoryGroup(groups)
	if slug == "" {
		slug = normalizeCategorySlug(name)
	}
	if categoryID <= 0 || name == "" || slug == "" {
		return Category{}, fmt.Errorf("category id and name are required")
	}

	var oldSlug string
	if err := s.db.QueryRow(ctx, `SELECT slug FROM categories WHERE id = $1`, categoryID).Scan(&oldSlug); err != nil {
		return Category{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Category{}, err
	}
	defer tx.Rollback(ctx)

	var category Category
	if err := tx.QueryRow(ctx, `
		UPDATE categories
		SET name = $2,
		    slug = $3,
		    sub2api_main_group_id = $4,
		    sub2api_main_group_name = $5,
		    sub2api_main_groups = $6::jsonb,
		    blocked_group_keywords = $7::jsonb,
		    updated_at = now()
		WHERE id = $1
		RETURNING id, name, slug, sub2api_main_group_id, sub2api_main_group_name, COALESCE(sub2api_main_groups, '[]'::jsonb), COALESCE(blocked_group_keywords, '[]'::jsonb), created_at, updated_at
	`, categoryID, name, slug, primary.ID, primary.Name, string(groupsJSON), string(blockedJSON)).Scan(
		&category.ID, &category.Name, &category.Slug, &category.Sub2APIMainGroupID, &category.Sub2APIMainGroupName,
		&groupsJSON, &blockedJSON,
		&category.CreatedAt, &category.UpdatedAt,
	); err != nil {
		return Category{}, err
	}
	category.Sub2APIMainGroups = normalizeCategoryGroupRefs(category.Sub2APIMainGroupID, category.Sub2APIMainGroupName, groupsJSON)
	category.BlockedGroupKeywords = blockedKeywords
	if oldSlug != slug {
		if _, err := tx.Exec(ctx, `UPDATE monitor_rules SET category = $2, updated_at = now() WHERE category = $1`, oldSlug, slug); err != nil {
			return Category{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE price_snapshots SET category = $2 WHERE category = $1`, oldSlug, slug); err != nil {
			return Category{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Category{}, err
	}
	return category, nil
}

func (s Store) DeleteCategory(ctx context.Context, categoryID int64) error {
	if categoryID <= 0 {
		return fmt.Errorf("invalid category id")
	}
	var slug string
	if err := s.db.QueryRow(ctx, `SELECT slug FROM categories WHERE id = $1`, categoryID).Scan(&slug); err != nil {
		return err
	}
	if slug == "other" {
		return fmt.Errorf("default category cannot be deleted")
	}
	var used int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM monitor_rules WHERE category = $1`, slug).Scan(&used); err != nil {
		return err
	}
	if used > 0 {
		return fmt.Errorf("category is used by %d monitor rule(s)", used)
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM categories WHERE id = $1`, categoryID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s Store) GetCategoryBySlug(ctx context.Context, slug string) (Category, error) {
	slug = normalizeCategorySlug(slug)
	if slug == "" {
		return Category{}, fmt.Errorf("category slug is required")
	}
	var category Category
	var groupsRaw []byte
	var blockedRaw []byte
	err := s.db.QueryRow(ctx, `
		SELECT id, name, slug, sub2api_main_group_id, sub2api_main_group_name, COALESCE(sub2api_main_groups, '[]'::jsonb), COALESCE(blocked_group_keywords, '[]'::jsonb), created_at, updated_at
		FROM categories
		WHERE slug = $1
	`, slug).Scan(
		&category.ID, &category.Name, &category.Slug, &category.Sub2APIMainGroupID, &category.Sub2APIMainGroupName,
		&groupsRaw, &blockedRaw,
		&category.CreatedAt, &category.UpdatedAt,
	)
	category.Sub2APIMainGroups = normalizeCategoryGroupRefs(category.Sub2APIMainGroupID, category.Sub2APIMainGroupName, groupsRaw)
	category.BlockedGroupKeywords = normalizeStringListFromJSON(blockedRaw)
	return category, err
}

func normalizeStringList(values []string) ([]string, []byte) {
	normalized := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == '，' || r == '\n' || r == '\r' || r == '\t'
		}) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			key := strings.ToLower(part)
			if seen[key] {
				continue
			}
			seen[key] = true
			normalized = append(normalized, part)
		}
	}
	data, _ := json.Marshal(normalized)
	return normalized, data
}

func normalizeStringListFromJSON(raw []byte) []string {
	var values []string
	_ = json.Unmarshal(raw, &values)
	normalized, _ := normalizeStringList(values)
	return normalized
}

func normalizeCategoryInputGroups(input CategoryInput) ([]Sub2APIGroupRef, []byte, error) {
	groups := make([]Sub2APIGroupRef, 0, len(input.Sub2APIMainGroups)+1)
	seen := map[string]bool{}
	for _, group := range input.Sub2APIMainGroups {
		group.Name = strings.TrimSpace(group.Name)
		if group.ID <= 0 && group.Name == "" {
			continue
		}
		key := fmt.Sprintf("%d:%s", group.ID, strings.ToLower(group.Name))
		if seen[key] {
			continue
		}
		seen[key] = true
		groups = append(groups, group)
	}
	if len(groups) == 0 {
		group := Sub2APIGroupRef{ID: input.Sub2APIMainGroupID, Name: strings.TrimSpace(input.Sub2APIMainGroupName)}
		if group.ID > 0 || group.Name != "" {
			groups = append(groups, group)
		}
	}
	data, err := json.Marshal(groups)
	if err != nil {
		return nil, nil, err
	}
	return groups, data, nil
}

func normalizeCategoryGroupRefs(groupID int64, groupName string, raw []byte) []Sub2APIGroupRef {
	var groups []Sub2APIGroupRef
	_ = json.Unmarshal(raw, &groups)
	normalized, data, err := normalizeCategoryInputGroups(CategoryInput{
		Sub2APIMainGroupID:   groupID,
		Sub2APIMainGroupName: groupName,
		Sub2APIMainGroups:    groups,
	})
	if err != nil || len(data) == 0 {
		return nil
	}
	return normalized
}

func primaryCategoryGroup(groups []Sub2APIGroupRef) Sub2APIGroupRef {
	if len(groups) == 0 {
		return Sub2APIGroupRef{}
	}
	return groups[0]
}

func (s Store) CreateRule(ctx context.Context, input RuleInput) (Rule, error) {
	input, err := s.normalizeRuleInput(ctx, input)
	if err != nil {
		return Rule{}, err
	}
	if err := s.ensureUniqueRule(ctx, input, 0); err != nil {
		return Rule{}, err
	}

	var rule Rule
	err = s.db.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO monitor_rules (
				source_type, site_id, sub2api_upstream_id, category, model_keyword, model_name, group_name, enabled,
				schedule_enabled, interval_minutes, next_run_at,
				sync_enabled, sync_base_group, sync_threshold_ratio, sub2api_group_name, sub2api_group_id
			)
			VALUES ($1, NULLIF($2, 0), $3, $4, $5, $6, $7, true, $8, $9::int, CASE WHEN $8 THEN COALESCE($15::timestamptz, now() + make_interval(mins => $9::int)) ELSE NULL END, $10, $11, NULLIF($12, 0), $13, $14)
			RETURNING id, source_type, COALESCE(site_id, 0) AS site_id, sub2api_upstream_id, category, model_keyword, model_name, group_name, enabled,
			          schedule_enabled, interval_minutes, next_run_at, last_scheduled_run_at,
			          sync_enabled, sync_base_group, sync_threshold_ratio, sub2api_group_name, sub2api_group_id,
			          last_sync_at, sync_status, sync_error,
			          checkin_enabled, checkin_status, checkin_reward, checkin_reward_unit, checkin_message, checkin_checked_at,
			          created_at, updated_at
		)
		SELECT r.id, r.source_type, r.site_id, COALESCE(s.name, ''), r.sub2api_upstream_id, COALESCE(u.name, ''),
		       CASE WHEN r.source_type = 'sub2api' THEN COALESCE(u.name, '') ELSE COALESCE(s.name, '') END AS source_name,
		       CASE WHEN r.source_type = 'sub2api' THEN COALESCE(u.base_url, '') ELSE COALESCE(s.base_url, '') END AS source_base_url,
		       CASE WHEN r.source_type = 'sub2api' THEN CASE WHEN trim(COALESCE(u.email, '')) <> '' THEN trim(u.email) WHEN trim(COALESCE(u.auth_token, '')) <> '' THEN 'token:' || left(md5(u.auth_token), 12) ELSE '' END ELSE COALESCE(s.username, '') END AS source_account,
		       r.category, COALESCE(c.name, r.category),
		       r.model_keyword, r.model_name, COALESCE(r.group_name, ''), r.enabled,
		       r.schedule_enabled, r.interval_minutes, r.next_run_at, r.last_scheduled_run_at,
		       r.sync_enabled, r.sync_base_group, r.sync_threshold_ratio, r.sub2api_group_name, r.sub2api_group_id,
		       r.last_sync_at, r.sync_status, r.sync_error,
		       NULL::double precision AS upstream_balance, '' AS balance_unit,
		       r.checkin_enabled, r.checkin_status, r.checkin_reward, r.checkin_reward_unit, r.checkin_message, r.checkin_checked_at,
		       r.created_at, r.updated_at
		FROM inserted r
		LEFT JOIN sites s ON s.id = r.site_id
		LEFT JOIN sub2api_upstreams u ON u.id = r.sub2api_upstream_id
		LEFT JOIN categories c ON c.slug = r.category
	`, input.SourceType, input.SiteID, input.Sub2APIUpstreamID, input.Category, input.ModelKeyword, input.ModelName, input.GroupName,
		input.ScheduleEnabled, input.IntervalMinutes, input.SyncEnabled, input.SyncBaseGroup,
		input.SyncThresholdRatio, input.Sub2APIGroupName, input.Sub2APIGroupID, input.InitialNextRunAt).Scan(
		&rule.ID, &rule.SourceType, &rule.SiteID, &rule.SiteName, &rule.Sub2APIUpstreamID, &rule.Sub2APIUpstreamName,
		&rule.SourceName, &rule.SourceBaseURL, &rule.SourceAccount, &rule.Category, &rule.CategoryName,
		&rule.ModelKeyword, &rule.ModelName, &rule.GroupName, &rule.Enabled,
		&rule.ScheduleEnabled, &rule.IntervalMinutes, &rule.NextRunAt, &rule.LastScheduledRunAt,
		&rule.SyncEnabled, &rule.SyncBaseGroup, &rule.SyncThresholdRatio, &rule.Sub2APIGroupName, &rule.Sub2APIGroupID,
		&rule.LastSyncAt, &rule.SyncStatus, &rule.SyncError, &rule.UpstreamBalance, &rule.BalanceUnit,
		&rule.CheckinEnabled, &rule.CheckinStatus, &rule.CheckinReward, &rule.CheckinRewardUnit, &rule.CheckinMessage, &rule.CheckinCheckedAt,
		&rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		return Rule{}, err
	}
	if err := s.syncRuleSnapshotsCategory(ctx, rule.ID, rule.Category); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

func (s Store) UpdateRule(ctx context.Context, ruleID int64, input RuleInput) (Rule, error) {
	if ruleID <= 0 {
		return Rule{}, fmt.Errorf("invalid rule id")
	}
	input, err := s.normalizeRuleInput(ctx, input)
	if err != nil {
		return Rule{}, err
	}
	if err := s.ensureUniqueRule(ctx, input, ruleID); err != nil {
		return Rule{}, err
	}

	var rule Rule
	err = s.db.QueryRow(ctx, `
		WITH updated AS (
			UPDATE monitor_rules r
			SET source_type = $2,
			    site_id = NULLIF($3, 0),
			    sub2api_upstream_id = $4,
			    category = $5,
			    model_keyword = $6,
			    model_name = $7,
			    group_name = $8,
			    enabled = $9,
			    schedule_enabled = $10,
			    interval_minutes = $11::int,
				    next_run_at = CASE
				      WHEN $10 THEN
				        CASE
				          WHEN r.schedule_enabled = false OR r.interval_minutes <> $11::int THEN now() + make_interval(mins => $11::int)
				          ELSE COALESCE(r.next_run_at, now())
				        END
				      ELSE NULL
			    END,
			    sync_enabled = $12,
			    sync_base_group = $13,
			    sync_threshold_ratio = NULLIF($14, 0),
			    sub2api_group_name = $15,
			    sub2api_group_id = $16,
			    updated_at = now()
			WHERE r.id = $1
			RETURNING r.*
		)
		SELECT r.id, r.source_type, COALESCE(r.site_id, 0), COALESCE(s.name, ''), r.sub2api_upstream_id, COALESCE(u.name, ''),
		       CASE WHEN r.source_type = 'sub2api' THEN COALESCE(u.name, '') ELSE COALESCE(s.name, '') END AS source_name,
		       CASE WHEN r.source_type = 'sub2api' THEN COALESCE(u.base_url, '') ELSE COALESCE(s.base_url, '') END AS source_base_url,
		       CASE WHEN r.source_type = 'sub2api' THEN CASE WHEN trim(COALESCE(u.email, '')) <> '' THEN trim(u.email) WHEN trim(COALESCE(u.auth_token, '')) <> '' THEN 'token:' || left(md5(u.auth_token), 12) ELSE '' END ELSE COALESCE(s.username, '') END AS source_account,
		       r.category, COALESCE(c.name, r.category),
		       r.model_keyword, r.model_name, COALESCE(r.group_name, ''), r.enabled,
		       r.schedule_enabled, r.interval_minutes, r.next_run_at, r.last_scheduled_run_at,
		       r.sync_enabled, r.sync_base_group, r.sync_threshold_ratio, r.sub2api_group_name, r.sub2api_group_id,
		       r.last_sync_at, r.sync_status, r.sync_error,
		       latest_price.upstream_balance, COALESCE(latest_price.balance_unit, ''),
		       r.checkin_enabled, r.checkin_status, r.checkin_reward, r.checkin_reward_unit, r.checkin_message, r.checkin_checked_at,
		       r.created_at, r.updated_at
		FROM updated r
		LEFT JOIN sites s ON s.id = r.site_id
		LEFT JOIN sub2api_upstreams u ON u.id = r.sub2api_upstream_id
		LEFT JOIN categories c ON c.slug = r.category
		LEFT JOIN LATERAL (
			SELECT p.upstream_balance, p.balance_unit
			FROM price_snapshots p
			WHERE p.rule_id = r.id
			  AND p.invalid = false
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT 1
		) latest_price ON true
	`, ruleID, input.SourceType, input.SiteID, input.Sub2APIUpstreamID, input.Category, input.ModelKeyword, input.ModelName, input.GroupName, input.Enabled,
		input.ScheduleEnabled, input.IntervalMinutes, input.SyncEnabled, input.SyncBaseGroup,
		input.SyncThresholdRatio, input.Sub2APIGroupName, input.Sub2APIGroupID).Scan(
		&rule.ID, &rule.SourceType, &rule.SiteID, &rule.SiteName, &rule.Sub2APIUpstreamID, &rule.Sub2APIUpstreamName,
		&rule.SourceName, &rule.SourceBaseURL, &rule.SourceAccount, &rule.Category, &rule.CategoryName,
		&rule.ModelKeyword, &rule.ModelName, &rule.GroupName, &rule.Enabled,
		&rule.ScheduleEnabled, &rule.IntervalMinutes, &rule.NextRunAt, &rule.LastScheduledRunAt,
		&rule.SyncEnabled, &rule.SyncBaseGroup, &rule.SyncThresholdRatio, &rule.Sub2APIGroupName, &rule.Sub2APIGroupID,
		&rule.LastSyncAt, &rule.SyncStatus, &rule.SyncError, &rule.UpstreamBalance, &rule.BalanceUnit,
		&rule.CheckinEnabled, &rule.CheckinStatus, &rule.CheckinReward, &rule.CheckinRewardUnit, &rule.CheckinMessage, &rule.CheckinCheckedAt,
		&rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		return Rule{}, err
	}
	if err := s.syncRuleSnapshotsCategory(ctx, rule.ID, rule.Category); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

func (s Store) BulkUpdateRules(ctx context.Context, input BulkRuleUpdateInput) ([]Rule, error) {
	ruleIDs, err := normalizeRuleIDs(input.RuleIDs)
	if err != nil {
		return nil, err
	}
	if !input.UpdateModelKeyword && !input.UpdateIntervalMinutes && !input.UpdateScheduleEnabled && !input.UpdateSyncEnabled {
		return nil, fmt.Errorf("请选择至少一个要批量修改的字段")
	}
	modelKeyword := strings.TrimSpace(input.ModelKeyword)
	if input.UpdateModelKeyword && modelKeyword == "" {
		return nil, fmt.Errorf("模型名称不能为空")
	}
	if input.UpdateIntervalMinutes && input.IntervalMinutes <= 0 {
		return nil, fmt.Errorf("监控间隔必须大于 0")
	}

	rules := make([]Rule, 0, len(ruleIDs))
	intendedInputs := make(map[int64]RuleInput, len(ruleIDs))
	intendedSignatures := map[string]int64{}
	for _, ruleID := range ruleIDs {
		rule, _, _, err := s.GetRuleWithSource(ctx, ruleID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("规则 #%d 不存在", ruleID)
			}
			return nil, err
		}
		nextInput := ruleInputFromRule(rule)
		if input.UpdateModelKeyword {
			nextInput.ModelKeyword = modelKeyword
			nextInput.ModelName = modelKeyword
		}
		if input.UpdateIntervalMinutes {
			nextInput.IntervalMinutes = input.IntervalMinutes
		}
		if input.UpdateScheduleEnabled {
			nextInput.ScheduleEnabled = input.ScheduleEnabled
		}
		if input.UpdateSyncEnabled {
			nextInput.SyncEnabled = input.SyncEnabled
		}
		normalizedInput, err := s.normalizeRuleInput(ctx, nextInput)
		if err != nil {
			return nil, fmt.Errorf("规则 #%d：%w", ruleID, err)
		}
		if input.UpdateModelKeyword {
			if err := s.ensureUniqueRule(ctx, normalizedInput, ruleID); err != nil {
				return nil, fmt.Errorf("规则 #%d：%w", ruleID, err)
			}
			signature := intendedRuleSignature(rule, normalizedInput)
			if previousID, ok := intendedSignatures[signature]; ok {
				return nil, fmt.Errorf("规则 #%d 和规则 #%d 修改后会重复", previousID, ruleID)
			}
			intendedSignatures[signature] = ruleID
		}
		intendedInputs[ruleID] = normalizedInput
		rules = append(rules, rule)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	for _, rule := range rules {
		nextInput := intendedInputs[rule.ID]
		if _, err := tx.Exec(ctx, `
			UPDATE monitor_rules
			SET model_keyword = $2,
			    model_name = $3,
			    schedule_enabled = $4,
			    interval_minutes = $5,
			    next_run_at = CASE
			      WHEN $4 THEN
			        CASE
			          WHEN schedule_enabled = false OR interval_minutes <> $5 THEN now() + make_interval(mins => $5)
			          ELSE COALESCE(next_run_at, now())
			        END
			      ELSE NULL
			    END,
			    sync_enabled = $6,
			    updated_at = now()
			WHERE id = $1
		`, rule.ID, nextInput.ModelKeyword, nextInput.ModelName, nextInput.ScheduleEnabled, nextInput.IntervalMinutes, nextInput.SyncEnabled); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return s.rulesByIDs(ctx, ruleIDs)
}

func (s Store) DisableAllRuleSync(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		UPDATE monitor_rules
		SET sync_enabled = false,
		    sync_status = CASE
		      WHEN sync_enabled THEN '主站 sub2api 同步开关未开启，已关闭规则同步'
		      ELSE sync_status
		    END,
		    sync_error = '',
		    updated_at = now()
		WHERE sync_enabled = true
	`)
	return err
}

func normalizeRuleIDs(rawIDs []int64) ([]int64, error) {
	seen := map[int64]bool{}
	ids := make([]int64, 0, len(rawIDs))
	for _, id := range rawIDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("请选择要编辑的监控规则")
	}
	return ids, nil
}

func ruleInputFromRule(rule Rule) RuleInput {
	return RuleInput{
		SourceType:         rule.SourceType,
		SiteID:             rule.SiteID,
		Sub2APIUpstreamID:  rule.Sub2APIUpstreamID,
		Category:           rule.Category,
		ModelKeyword:       rule.ModelKeyword,
		ModelName:          firstNonEmpty(rule.ModelName, rule.ModelKeyword),
		GroupName:          rule.GroupName,
		Enabled:            rule.Enabled,
		ScheduleEnabled:    rule.ScheduleEnabled,
		IntervalMinutes:    rule.IntervalMinutes,
		SyncEnabled:        rule.SyncEnabled,
		SyncBaseGroup:      rule.SyncBaseGroup,
		Sub2APIGroupName:   rule.Sub2APIGroupName,
		Sub2APIGroupID:     rule.Sub2APIGroupID,
		SyncThresholdRatio: floatValueFromPtr(rule.SyncThresholdRatio),
	}
}

func intendedRuleSignature(rule Rule, input RuleInput) string {
	sourceType := strings.ToLower(strings.TrimSpace(input.SourceType))
	category := normalizeCategorySlug(input.Category)
	model := strings.ToLower(strings.TrimSpace(input.ModelKeyword))
	if sourceType == RuleSourceSub2API {
		return strings.Join([]string{
			sourceType,
			strings.ToLower(strings.TrimRight(normalizeBaseURL(rule.SourceBaseURL), "/")),
			strings.ToLower(strings.TrimSpace(rule.SourceAccount)),
			category,
			model,
		}, "\x00")
	}
	return strings.Join([]string{
		sourceType,
		strconv.FormatInt(input.SiteID, 10),
		strconv.FormatInt(input.Sub2APIUpstreamID, 10),
		category,
		model,
	}, "\x00")
}

func (s Store) ensureUniqueRule(ctx context.Context, input RuleInput, excludeID int64) error {
	var existingID int64
	var err error
	if input.SourceType == RuleSourceSub2API {
		err = s.db.QueryRow(ctx, `
			WITH target AS (
				SELECT lower(regexp_replace(trim(COALESCE(base_url, '')), '/+$', '')) AS base_key,
				       lower(trim(CASE WHEN trim(COALESCE(email, '')) <> '' THEN trim(email) WHEN trim(COALESCE(auth_token, '')) <> '' THEN 'token:' || left(md5(auth_token), 12) ELSE '' END)) AS account_key
				FROM sub2api_upstreams
				WHERE id = $1
			)
			SELECT r.id
			FROM monitor_rules r
			JOIN sub2api_upstreams u ON u.id = r.sub2api_upstream_id
			CROSS JOIN target t
			WHERE r.source_type = 'sub2api'
			  AND lower(regexp_replace(trim(COALESCE(u.base_url, '')), '/+$', '')) = t.base_key
			  AND lower(trim(CASE WHEN trim(COALESCE(u.email, '')) <> '' THEN trim(u.email) WHEN trim(COALESCE(u.auth_token, '')) <> '' THEN 'token:' || left(md5(u.auth_token), 12) ELSE '' END)) = t.account_key
			  AND r.category = $2
			  AND lower(trim(r.model_keyword)) = lower(trim($3::text))
			  AND ($4::bigint = 0 OR r.id <> $4)
			ORDER BY r.id
			LIMIT 1
		`, input.Sub2APIUpstreamID, input.Category, input.ModelKeyword, excludeID).Scan(&existingID)
	} else {
		err = s.db.QueryRow(ctx, `
			SELECT id
			FROM monitor_rules
			WHERE source_type = $1
			  AND COALESCE(site_id, 0) = $2
			  AND COALESCE(sub2api_upstream_id, 0) = $3
			  AND category = $4
			  AND lower(trim(model_keyword)) = lower(trim($5::text))
			  AND ($6::bigint = 0 OR id <> $6)
			ORDER BY id
			LIMIT 1
		`, input.SourceType, input.SiteID, input.Sub2APIUpstreamID, input.Category, input.ModelKeyword, excludeID).Scan(&existingID)
	}
	if err == nil {
		return fmt.Errorf("相同站点、登录账号、分类和模型的监控规则已存在，请勿重复添加")
	}
	if err == pgx.ErrNoRows {
		return nil
	}
	return err
}

func (s Store) syncRuleSnapshotsCategory(ctx context.Context, ruleID int64, category string) error {
	category = normalizeCategorySlug(category)
	if ruleID <= 0 || category == "" {
		return nil
	}
	if _, err := s.db.Exec(ctx, `
		DELETE FROM price_snapshots p
		USING (
		  SELECT id,
		         row_number() OVER (
		           PARTITION BY COALESCE(source_type, 'newapi'), lower(regexp_replace(trim(source_base_url), '/+$', '')), lower(trim(source_account)),
		                        $2::text, model_name, lower(trim(group_name))
		           ORDER BY CASE WHEN category = $2 THEN 0 ELSE 1 END, created_at DESC, id DESC
		         ) AS duplicate_rank
		  FROM price_snapshots
		  WHERE rule_id = $1
		) ranked
		WHERE p.id = ranked.id
		  AND ranked.duplicate_rank > 1
	`, ruleID, category); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `
		UPDATE price_snapshots
		SET category = $2
		WHERE rule_id = $1
		  AND category <> $2
	`, ruleID, category)
	return err
}

func (s Store) DeleteRule(ctx context.Context, ruleID int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM monitor_rules WHERE id = $1`, ruleID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s Store) normalizeRuleInput(ctx context.Context, input RuleInput) (RuleInput, error) {
	input.SourceType = strings.ToLower(strings.TrimSpace(input.SourceType))
	if input.SourceType == "" {
		input.SourceType = RuleSourceNewAPI
	}
	input.Category = normalizeCategorySlug(input.Category)
	input.ModelKeyword = strings.TrimSpace(input.ModelKeyword)
	if input.ModelKeyword == "" {
		input.ModelKeyword = strings.TrimSpace(input.ModelName)
	}
	input.ModelName = strings.TrimSpace(input.ModelName)
	input.GroupName = strings.TrimSpace(input.GroupName)
	input.SyncBaseGroup = strings.TrimSpace(input.SyncBaseGroup)
	if input.SyncBaseGroup == "" {
		input.SyncBaseGroup = input.GroupName
	}
	if input.SyncThresholdRatio < 0 {
		input.SyncThresholdRatio = 0
	}
	input.Sub2APIGroupName = strings.TrimSpace(input.Sub2APIGroupName)
	if input.IntervalMinutes <= 0 {
		input.IntervalMinutes = 15
	}
	if input.ModelKeyword == "" {
		return RuleInput{}, fmt.Errorf("model keyword is required")
	}
	switch input.SourceType {
	case RuleSourceNewAPI:
		input.Sub2APIUpstreamID = 0
		if input.SiteID <= 0 {
			return RuleInput{}, fmt.Errorf("newapi source site is required")
		}
	case RuleSourceSub2API:
		input.SiteID = 0
		if input.Sub2APIUpstreamID <= 0 {
			return RuleInput{}, fmt.Errorf("sub2api source site is required")
		}
	default:
		return RuleInput{}, fmt.Errorf("unsupported source type %q", input.SourceType)
	}
	if input.Category == "" {
		input.Category = "other"
	}
	if input.ModelName == "" {
		input.ModelName = input.ModelKeyword
	}
	var categoryExists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM categories WHERE slug = $1)`, input.Category).Scan(&categoryExists); err != nil {
		return RuleInput{}, err
	}
	if !categoryExists {
		return RuleInput{}, fmt.Errorf("category %q does not exist", input.Category)
	}
	if input.Sub2APIUpstreamID > 0 {
		var upstreamExists bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sub2api_upstreams WHERE id = $1)`, input.Sub2APIUpstreamID).Scan(&upstreamExists); err != nil {
			return RuleInput{}, err
		}
		if !upstreamExists {
			return RuleInput{}, fmt.Errorf("sub2api upstream %d does not exist", input.Sub2APIUpstreamID)
		}
	}
	return input, nil
}

func (s Store) ListRules(ctx context.Context) ([]Rule, error) {
	rows, err := s.db.Query(ctx, `
		SELECT r.id, COALESCE(r.source_type, 'newapi'), COALESCE(r.site_id, 0), COALESCE(s.name, ''), r.sub2api_upstream_id, COALESCE(u.name, ''),
		       CASE WHEN COALESCE(r.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.name, '') ELSE COALESCE(s.name, '') END AS source_name,
		       CASE WHEN COALESCE(r.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.base_url, '') ELSE COALESCE(s.base_url, '') END AS source_base_url,
		       CASE WHEN COALESCE(r.source_type, 'newapi') = 'sub2api' THEN CASE WHEN trim(COALESCE(u.email, '')) <> '' THEN trim(u.email) WHEN trim(COALESCE(u.auth_token, '')) <> '' THEN 'token:' || left(md5(u.auth_token), 12) ELSE '' END ELSE COALESCE(s.username, '') END AS source_account,
		       r.category, COALESCE(c.name, r.category),
		       r.model_keyword, r.model_name, COALESCE(r.group_name, ''), r.enabled,
		       r.schedule_enabled, r.interval_minutes, r.next_run_at, r.last_scheduled_run_at,
		       r.sync_enabled, r.sync_base_group, r.sync_threshold_ratio, r.sub2api_group_name, r.sub2api_group_id,
		       r.last_sync_at, r.sync_status, r.sync_error,
		       latest_price.upstream_balance, COALESCE(latest_price.balance_unit, ''),
		       r.checkin_enabled, r.checkin_status, r.checkin_reward, r.checkin_reward_unit, r.checkin_message, r.checkin_checked_at,
		       r.created_at, r.updated_at
		FROM monitor_rules r
		LEFT JOIN sites s ON s.id = r.site_id
		LEFT JOIN sub2api_upstreams u ON u.id = r.sub2api_upstream_id
		LEFT JOIN categories c ON c.slug = r.category
		LEFT JOIN LATERAL (
			SELECT p.input_price, p.request_price, p.output_price, p.group_ratio, p.upstream_balance, p.balance_unit
			FROM price_snapshots p
			WHERE p.rule_id = r.id
			  AND p.invalid = false
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT 1
		) latest_price ON true
		ORDER BY CASE r.category WHEN 'codex' THEN 0 WHEN 'claud' THEN 1 WHEN 'other' THEN 99 ELSE 2 END,
		         COALESCE(c.name, r.category),
		         COALESCE(latest_price.input_price, latest_price.request_price, latest_price.output_price) ASC NULLS LAST,
		         latest_price.output_price ASC NULLS LAST,
		         latest_price.group_ratio ASC NULLS LAST,
		         r.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var rule Rule
		if err := rows.Scan(
			&rule.ID, &rule.SourceType, &rule.SiteID, &rule.SiteName, &rule.Sub2APIUpstreamID, &rule.Sub2APIUpstreamName,
			&rule.SourceName, &rule.SourceBaseURL, &rule.SourceAccount, &rule.Category, &rule.CategoryName,
			&rule.ModelKeyword, &rule.ModelName, &rule.GroupName,
			&rule.Enabled, &rule.ScheduleEnabled, &rule.IntervalMinutes, &rule.NextRunAt, &rule.LastScheduledRunAt,
			&rule.SyncEnabled, &rule.SyncBaseGroup, &rule.SyncThresholdRatio, &rule.Sub2APIGroupName, &rule.Sub2APIGroupID,
			&rule.LastSyncAt, &rule.SyncStatus, &rule.SyncError, &rule.UpstreamBalance, &rule.BalanceUnit,
			&rule.CheckinEnabled, &rule.CheckinStatus, &rule.CheckinReward, &rule.CheckinRewardUnit, &rule.CheckinMessage, &rule.CheckinCheckedAt,
			&rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (s Store) rulesByIDs(ctx context.Context, ids []int64) ([]Rule, error) {
	if len(ids) == 0 {
		return []Rule{}, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT r.id, COALESCE(r.source_type, 'newapi'), COALESCE(r.site_id, 0), COALESCE(s.name, ''), r.sub2api_upstream_id, COALESCE(u.name, ''),
		       CASE WHEN COALESCE(r.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.name, '') ELSE COALESCE(s.name, '') END AS source_name,
		       CASE WHEN COALESCE(r.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.base_url, '') ELSE COALESCE(s.base_url, '') END AS source_base_url,
		       CASE WHEN COALESCE(r.source_type, 'newapi') = 'sub2api' THEN CASE WHEN trim(COALESCE(u.email, '')) <> '' THEN trim(u.email) WHEN trim(COALESCE(u.auth_token, '')) <> '' THEN 'token:' || left(md5(u.auth_token), 12) ELSE '' END ELSE COALESCE(s.username, '') END AS source_account,
		       r.category, COALESCE(c.name, r.category),
		       r.model_keyword, r.model_name, COALESCE(r.group_name, ''), r.enabled,
		       r.schedule_enabled, r.interval_minutes, r.next_run_at, r.last_scheduled_run_at,
		       r.sync_enabled, r.sync_base_group, r.sync_threshold_ratio, r.sub2api_group_name, r.sub2api_group_id,
		       r.last_sync_at, r.sync_status, r.sync_error,
		       latest_price.upstream_balance, COALESCE(latest_price.balance_unit, ''),
		       r.checkin_enabled, r.checkin_status, r.checkin_reward, r.checkin_reward_unit, r.checkin_message, r.checkin_checked_at,
		       r.created_at, r.updated_at
		FROM monitor_rules r
		LEFT JOIN sites s ON s.id = r.site_id
		LEFT JOIN sub2api_upstreams u ON u.id = r.sub2api_upstream_id
		LEFT JOIN categories c ON c.slug = r.category
		LEFT JOIN LATERAL (
			SELECT p.upstream_balance, p.balance_unit
			FROM price_snapshots p
			WHERE p.rule_id = r.id
			  AND p.invalid = false
			ORDER BY p.created_at DESC, p.id DESC
			LIMIT 1
		) latest_price ON true
		WHERE r.id = ANY($1)
		ORDER BY array_position($1, r.id)
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var rule Rule
		if err := rows.Scan(
			&rule.ID, &rule.SourceType, &rule.SiteID, &rule.SiteName, &rule.Sub2APIUpstreamID, &rule.Sub2APIUpstreamName,
			&rule.SourceName, &rule.SourceBaseURL, &rule.SourceAccount, &rule.Category, &rule.CategoryName,
			&rule.ModelKeyword, &rule.ModelName, &rule.GroupName,
			&rule.Enabled, &rule.ScheduleEnabled, &rule.IntervalMinutes, &rule.NextRunAt, &rule.LastScheduledRunAt,
			&rule.SyncEnabled, &rule.SyncBaseGroup, &rule.SyncThresholdRatio, &rule.Sub2APIGroupName, &rule.Sub2APIGroupID,
			&rule.LastSyncAt, &rule.SyncStatus, &rule.SyncError, &rule.UpstreamBalance, &rule.BalanceUnit,
			&rule.CheckinEnabled, &rule.CheckinStatus, &rule.CheckinReward, &rule.CheckinRewardUnit, &rule.CheckinMessage, &rule.CheckinCheckedAt,
			&rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(rules) != len(ids) {
		return nil, fmt.Errorf("部分规则不存在或已被删除")
	}
	return rules, nil
}

func (s Store) GetRuleWithSource(ctx context.Context, ruleID int64) (Rule, Site, Sub2APIUpstream, error) {
	var rule Rule
	var site Site
	var upstream Sub2APIUpstream
	err := s.db.QueryRow(ctx, `
		SELECT
			r.id, COALESCE(r.source_type, 'newapi'), COALESCE(r.site_id, 0), COALESCE(s.name, ''), r.sub2api_upstream_id, COALESCE(u.name, ''),
			CASE WHEN COALESCE(r.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.name, '') ELSE COALESCE(s.name, '') END AS source_name,
			CASE WHEN COALESCE(r.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.base_url, '') ELSE COALESCE(s.base_url, '') END AS source_base_url,
			CASE WHEN COALESCE(r.source_type, 'newapi') = 'sub2api' THEN CASE WHEN trim(COALESCE(u.email, '')) <> '' THEN trim(u.email) WHEN trim(COALESCE(u.auth_token, '')) <> '' THEN 'token:' || left(md5(u.auth_token), 12) ELSE '' END ELSE COALESCE(s.username, '') END AS source_account,
			r.category, COALESCE(c.name, r.category),
			r.model_keyword, r.model_name, COALESCE(r.group_name, ''), r.enabled,
			r.schedule_enabled, r.interval_minutes, r.next_run_at, r.last_scheduled_run_at,
			r.sync_enabled, r.sync_base_group, r.sync_threshold_ratio, r.sub2api_group_name, r.sub2api_group_id,
			r.last_sync_at, r.sync_status, r.sync_error,
			r.checkin_enabled, r.checkin_status, r.checkin_reward, r.checkin_reward_unit, r.checkin_message, r.checkin_checked_at,
			r.created_at, r.updated_at,
			COALESCE(s.id, 0), COALESCE(s.name, ''), COALESCE(s.base_url, ''), COALESCE(s.username, ''), COALESCE(s.password, ''), COALESCE(s.totp_code, ''), COALESCE(s.user_id, 0), COALESCE(s.access_token, ''), COALESCE(s.cookie_jar, ''), COALESCE(s.last_error, ''), s.last_run_at, COALESCE(s.created_at, now()), COALESCE(s.updated_at, now()),
			COALESCE(u.id, 0), COALESCE(u.name, ''), COALESCE(u.base_url, ''), COALESCE(u.email, ''), COALESCE(u.password, ''), COALESCE(u.auth_token, ''), COALESCE(u.totp_code, ''), COALESCE(u.cookie_jar, ''), COALESCE(u.last_error, ''), u.last_check_at, COALESCE(u.created_at, now()), COALESCE(u.updated_at, now())
		FROM monitor_rules r
		LEFT JOIN sites s ON s.id = r.site_id
		LEFT JOIN sub2api_upstreams u ON u.id = r.sub2api_upstream_id
		LEFT JOIN categories c ON c.slug = r.category
		WHERE r.id = $1
	`, ruleID).Scan(
		&rule.ID, &rule.SourceType, &rule.SiteID, &rule.SiteName, &rule.Sub2APIUpstreamID, &rule.Sub2APIUpstreamName,
		&rule.SourceName, &rule.SourceBaseURL, &rule.SourceAccount, &rule.Category, &rule.CategoryName,
		&rule.ModelKeyword, &rule.ModelName, &rule.GroupName, &rule.Enabled,
		&rule.ScheduleEnabled, &rule.IntervalMinutes, &rule.NextRunAt, &rule.LastScheduledRunAt,
		&rule.SyncEnabled, &rule.SyncBaseGroup, &rule.SyncThresholdRatio, &rule.Sub2APIGroupName, &rule.Sub2APIGroupID,
		&rule.LastSyncAt, &rule.SyncStatus, &rule.SyncError,
		&rule.CheckinEnabled, &rule.CheckinStatus, &rule.CheckinReward, &rule.CheckinRewardUnit, &rule.CheckinMessage, &rule.CheckinCheckedAt,
		&rule.CreatedAt, &rule.UpdatedAt,
		&site.ID, &site.Name, &site.BaseURL, &site.Username, &site.Password, &site.TOTPCode, &site.UserID, &site.AccessToken, &site.CookieJar, &site.LastError, &site.LastRunAt, &site.CreatedAt, &site.UpdatedAt,
		&upstream.ID, &upstream.Name, &upstream.BaseURL, &upstream.Email, &upstream.Password, &upstream.AuthToken, &upstream.TOTPCode, &upstream.CookieJar, &upstream.LastError, &upstream.LastCheckAt, &upstream.CreatedAt, &upstream.UpdatedAt,
	)
	if err != nil {
		return Rule{}, Site{}, Sub2APIUpstream{}, err
	}
	return rule, site, upstream, nil
}

func (s Store) EnabledRuleIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.db.Query(ctx, `SELECT id FROM monitor_rules WHERE enabled = true ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s Store) DueRuleIDs(ctx context.Context, now time.Time, limit int) ([]int64, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id
		FROM monitor_rules
		WHERE enabled = true
		  AND schedule_enabled = true
		  AND (next_run_at IS NULL OR next_run_at <= $1)
		ORDER BY COALESCE(next_run_at, created_at), id
		LIMIT $2
	`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s Store) ScheduledRuleIDs(ctx context.Context, limit int) ([]int64, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.db.Query(ctx, `
		SELECT id
		FROM monitor_rules
		WHERE enabled = true
		  AND schedule_enabled = true
		ORDER BY id
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s Store) MarkRuleScheduled(ctx context.Context, ruleID int64, runAt time.Time, intervalMinutes int) error {
	if intervalMinutes <= 0 {
		intervalMinutes = 15
	}
	nextRunAt := nextScheduledRunAt(runAt, intervalMinutes)
	_, err := s.db.Exec(ctx, `
		UPDATE monitor_rules
		SET last_scheduled_run_at = $2,
		    next_run_at = CASE
		      WHEN schedule_enabled THEN $3::timestamptz
		      ELSE NULL
		    END,
		    updated_at = now()
		WHERE id = $1
	`, ruleID, runAt, nextRunAt)
	return err
}

func (s Store) StaggerRules(ctx context.Context, sourceType string, category string, modelKeyword string, intervalMinutes int, base time.Time) (int64, error) {
	sourceType = strings.ToLower(strings.TrimSpace(sourceType))
	category = normalizeCategorySlug(category)
	modelKeyword = strings.TrimSpace(modelKeyword)
	if category == "" || modelKeyword == "" {
		return 0, nil
	}
	if intervalMinutes <= 0 {
		intervalMinutes = 15
	}
	args := []any{category, modelKeyword, intervalMinutes, base}
	sourceFilter := ""
	if sourceType == RuleSourceNewAPI || sourceType == RuleSourceSub2API {
		args = append(args, sourceType)
		sourceFilter = fmt.Sprintf("AND COALESCE(source_type, 'newapi') = $%d", len(args))
	}
	tag, err := s.db.Exec(ctx, `
		WITH target AS (
			SELECT id,
			       row_number() OVER (ORDER BY COALESCE(next_run_at, created_at), id) - 1 AS offset_index,
			       count(*) OVER () AS total_count
			FROM monitor_rules
			WHERE enabled = true
			  AND schedule_enabled = true
			  AND category = $1
			  AND lower(trim(model_keyword)) = lower(trim($2::text))
			  `+sourceFilter+`
		)
		UPDATE monitor_rules r
		SET next_run_at = $4::timestamptz
		  + interval '1 minute'
		  + greatest(
		      interval '1 minute',
		      make_interval(secs => (($3::double precision * 60) / greatest(target.total_count, 1))::int)
		    ) * target.offset_index,
		    updated_at = now()
		FROM target
		WHERE r.id = target.id
	`, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func nextScheduledRunAt(runAt time.Time, intervalMinutes int) time.Time {
	if intervalMinutes <= 0 {
		intervalMinutes = 15
	}
	return runAt.Add(time.Duration(intervalMinutes) * time.Minute)
}

func (s Store) RestoreRuleAfterManualRun(ctx context.Context, ruleID int64) error {
	_, err := s.db.Exec(ctx, `
		UPDATE monitor_rules
		SET enabled = true,
		    schedule_enabled = true,
		    next_run_at = now() + make_interval(mins => CASE WHEN interval_minutes > 0 THEN interval_minutes ELSE 15 END),
		    sync_status = CASE
		      WHEN sync_status LIKE 'paused after %' OR sync_status LIKE '已暂停：%' OR sync_status = 'error' OR sync_status = '同步失败' OR sync_status LIKE 'skip low balance:%' OR sync_status LIKE '跳过余额不足：%' THEN '手动运行成功'
		      ELSE sync_status
		    END,
		    sync_error = '',
		    sync_failure_count = 0,
		    sync_failure_signature = '',
		    updated_at = now()
		WHERE id = $1
	`, ruleID)
	return err
}

func (s Store) UpdateRuleSyncStatus(ctx context.Context, ruleID int64, status string, errText string) error {
	status = localizeErrorText(status)
	errText = localizeErrorText(errText)
	_, err := s.db.Exec(ctx, `
		UPDATE monitor_rules
		SET last_sync_at = CASE WHEN $2 <> '' THEN now() ELSE last_sync_at END,
		    sync_status = $2,
		    sync_error = $3,
		    updated_at = now()
		WHERE id = $1
	`, ruleID, status, errText)
	return err
}

func (s Store) UpdateRuleCheckinStatus(ctx context.Context, ruleID int64, result CheckinResult) error {
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = "unknown"
	}
	unit := strings.TrimSpace(result.Unit)
	if unit == "" {
		unit = "usd"
	}
	checkedAt := result.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = time.Now()
	}
	_, err := s.db.Exec(ctx, `
		UPDATE monitor_rules
		SET checkin_enabled = $2,
		    checkin_status = $3,
		    checkin_reward = $4,
		    checkin_reward_unit = $5,
		    checkin_message = $6,
		    checkin_checked_at = $7,
		    updated_at = now()
		WHERE id = $1
	`, ruleID, result.Enabled, status, result.Reward, unit, strings.TrimSpace(result.Message), checkedAt)
	return err
}

func (s Store) UpdateRuleSyncSuccess(ctx context.Context, ruleID int64, status string, signature string) error {
	status = localizeErrorText(status)
	_, err := s.db.Exec(ctx, `
		UPDATE monitor_rules
		SET last_sync_at = now(),
		    sync_status = $2,
		    sync_error = '',
		    sync_signature = $3,
		    sync_failure_count = 0,
		    sync_failure_signature = '',
		    updated_at = now()
		WHERE id = $1
	`, ruleID, status, signature)
	return err
}

func (s Store) RecordRuleSyncFailure(ctx context.Context, ruleID int64, status string, errText string, failureSignature string, pauseAfter int) (int, bool, bool, error) {
	if pauseAfter <= 0 {
		pauseAfter = 3
	}
	status = localizeErrorText(status)
	errText = localizeErrorText(errText)
	var failureCount int
	var previousSignature string
	if err := s.db.QueryRow(ctx, `
		SELECT sync_failure_count, COALESCE(sync_failure_signature, '')
		FROM monitor_rules
		WHERE id = $1
	`, ruleID).Scan(&failureCount, &previousSignature); err != nil {
		return 0, false, false, err
	}
	failureSignature = strings.TrimSpace(failureSignature)
	if failureSignature == "" {
		failureSignature = strings.TrimSpace(errText)
	}
	failureCount++
	paused := failureCount >= pauseAfter
	shouldNotify := failureSignature == "" || failureSignature != previousSignature || failureCount == 1
	if paused {
		status = fmt.Sprintf("已暂停：连续 %d 次同步失败", failureCount)
	}

	_, err := s.db.Exec(ctx, `
		UPDATE monitor_rules
		SET enabled = CASE WHEN $6 THEN false ELSE enabled END,
		    schedule_enabled = CASE WHEN $6 THEN false ELSE schedule_enabled END,
		    next_run_at = CASE WHEN $6 THEN NULL ELSE next_run_at END,
		    last_sync_at = now(),
		    sync_status = $2,
		    sync_error = $3,
		    sync_failure_count = $4,
		    sync_failure_signature = $5,
		    updated_at = now()
		WHERE id = $1
	`, ruleID, status, errText, failureCount, failureSignature, paused)
	if err != nil {
		return failureCount, paused, false, err
	}
	return failureCount, paused, shouldNotify, nil
}

func (s Store) RuleSyncSignature(ctx context.Context, ruleID int64) (string, error) {
	var signature string
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(sync_signature, '')
		FROM monitor_rules
		WHERE id = $1
	`, ruleID).Scan(&signature)
	return signature, err
}

func (s Store) RecordLowBalanceNotification(ctx context.Context, signature string) (bool, error) {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return false, nil
	}
	var inserted bool
	err := s.db.QueryRow(ctx, `
		INSERT INTO low_balance_notifications (signature)
		VALUES ($1)
		ON CONFLICT (signature) DO NOTHING
		RETURNING true
	`, signature).Scan(&inserted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return inserted, nil
}

func (s Store) InsertSnapshot(ctx context.Context, snapshot PriceSnapshot) (PriceSnapshot, error) {
	if strings.TrimSpace(snapshot.SourceType) == "" {
		snapshot.SourceType = RuleSourceNewAPI
	}
	err := s.db.QueryRow(ctx, `
		INSERT INTO price_snapshots (
			rule_id, source_type, site_id, sub2api_upstream_id, source_base_url, source_account, category, model_keyword, model_name, group_name, group_desc, quota_type, group_ratio,
			input_price, output_price, cache_read_price, cache_write_price, request_price, upstream_balance, balance_unit, online_topup_enabled, recharge_multiplier, raw
		)
		VALUES ($1, $2, NULLIF($3, 0), $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
		ON CONFLICT (
			COALESCE(source_type, 'newapi'),
			lower(regexp_replace(trim(source_base_url), '/+$', '')),
			lower(trim(source_account)),
			category,
			model_name,
			lower(trim(group_name))
		)
		DO UPDATE SET
			rule_id = EXCLUDED.rule_id,
			site_id = EXCLUDED.site_id,
			sub2api_upstream_id = EXCLUDED.sub2api_upstream_id,
			source_base_url = EXCLUDED.source_base_url,
			source_account = EXCLUDED.source_account,
			model_keyword = EXCLUDED.model_keyword,
			group_name = EXCLUDED.group_name,
			group_desc = EXCLUDED.group_desc,
			quota_type = EXCLUDED.quota_type,
			group_ratio = EXCLUDED.group_ratio,
			input_price = EXCLUDED.input_price,
			output_price = EXCLUDED.output_price,
			cache_read_price = EXCLUDED.cache_read_price,
			cache_write_price = EXCLUDED.cache_write_price,
			request_price = EXCLUDED.request_price,
			upstream_balance = EXCLUDED.upstream_balance,
			balance_unit = EXCLUDED.balance_unit,
			online_topup_enabled = EXCLUDED.online_topup_enabled,
			recharge_multiplier = EXCLUDED.recharge_multiplier,
			invalid = false,
			invalid_reason = '',
			invalid_at = NULL,
			raw = EXCLUDED.raw,
			created_at = now()
		RETURNING id, created_at
	`,
		snapshot.RuleID, snapshot.SourceType, snapshot.SiteID, snapshot.Sub2APIUpstreamID, normalizeBaseURL(snapshot.SiteBaseURL), strings.TrimSpace(snapshot.SourceAccount), normalizeCategorySlug(snapshot.Category), snapshot.ModelKeyword,
		snapshot.ModelName, snapshot.GroupName, snapshot.GroupDesc,
		snapshot.QuotaType, snapshot.GroupRatio, snapshot.InputPrice, snapshot.OutputPrice,
		snapshot.CacheReadPrice, snapshot.CacheWritePrice, snapshot.RequestPrice, snapshot.UpstreamBalance, snapshot.BalanceUnit, snapshot.OnlineTopupEnabled, snapshot.RechargeMultiplier, string(snapshot.Raw),
	).Scan(&snapshot.ID, &snapshot.CreatedAt)
	return snapshot, err
}

func (s Store) MarkMissingSnapshotGroupsInvalid(ctx context.Context, ruleID int64, modelName string, activeGroups []string, reason string) error {
	modelName = strings.TrimSpace(modelName)
	if ruleID <= 0 || modelName == "" {
		return nil
	}
	groupSet := make(map[string]struct{}, len(activeGroups))
	for _, group := range activeGroups {
		group = strings.ToLower(strings.TrimSpace(group))
		if group != "" {
			groupSet[group] = struct{}{}
		}
	}
	args := []any{ruleID, modelName, strings.TrimSpace(reason)}
	filter := ""
	if len(groupSet) > 0 {
		groups := make([]string, 0, len(groupSet))
		for group := range groupSet {
			groups = append(groups, group)
		}
		sort.Strings(groups)
		args = append(args, groups)
		filter = fmt.Sprintf("AND NOT (lower(trim(group_name)) = ANY($%d::text[]))", len(args))
	}
	_, err := s.db.Exec(ctx, `
		UPDATE price_snapshots
		SET invalid = true,
		    invalid_reason = CASE WHEN $3 = '' THEN 'upstream group disappeared' ELSE $3 END,
		    invalid_at = COALESCE(invalid_at, now())
		WHERE rule_id = $1
		  AND lower(trim(model_name)) = lower(trim($2))
		  AND invalid = false
		  `+filter, args...)
	return err
}

func (s Store) MarkCategoryMismatchedSnapshotsInvalid(ctx context.Context, reason string) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE price_snapshots p
		SET invalid = true,
		    invalid_reason = CASE WHEN $1 = '' THEN 'snapshot group does not match rule category' ELSE $1 END,
		    invalid_at = COALESCE(invalid_at, now())
		FROM monitor_rules r
		WHERE r.id = p.rule_id
		  AND p.invalid = false
		  AND (
		    (
		      (lower(r.category) LIKE '%claud%' OR lower(r.category) LIKE '%anthropic%' OR lower(r.model_keyword) LIKE '%claude%' OR lower(r.model_keyword) LIKE '%anthropic%')
		      AND (lower(COALESCE(p.group_name, '') || ' ' || COALESCE(p.group_desc, '')) LIKE '%codex%' OR lower(COALESCE(p.group_name, '') || ' ' || COALESCE(p.group_desc, '')) LIKE '%openai%' OR lower(COALESCE(p.group_name, '') || ' ' || COALESCE(p.group_desc, '')) LIKE '%gpt%')
		      AND NOT (lower(COALESCE(p.group_name, '') || ' ' || COALESCE(p.group_desc, '')) LIKE '%claude%' OR lower(COALESCE(p.group_name, '') || ' ' || COALESCE(p.group_desc, '')) LIKE '%claud%' OR lower(COALESCE(p.group_name, '') || ' ' || COALESCE(p.group_desc, '')) LIKE '%anthropic%')
		    )
		    OR
		    (
		      (lower(r.category) LIKE '%codex%' OR lower(r.category) LIKE '%openai%' OR lower(r.category) LIKE '%gpt%' OR lower(r.model_keyword) LIKE '%gpt%' OR lower(r.model_keyword) LIKE '%codex%')
		      AND (lower(COALESCE(p.group_name, '') || ' ' || COALESCE(p.group_desc, '')) LIKE '%claude%' OR lower(COALESCE(p.group_name, '') || ' ' || COALESCE(p.group_desc, '')) LIKE '%claud%' OR lower(COALESCE(p.group_name, '') || ' ' || COALESCE(p.group_desc, '')) LIKE '%anthropic%')
		      AND NOT (lower(COALESCE(p.group_name, '') || ' ' || COALESCE(p.group_desc, '')) LIKE '%codex%' OR lower(COALESCE(p.group_name, '') || ' ' || COALESCE(p.group_desc, '')) LIKE '%openai%' OR lower(COALESCE(p.group_name, '') || ' ' || COALESCE(p.group_desc, '')) LIKE '%gpt%')
		    )
		  )
	`, strings.TrimSpace(reason))
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s Store) PruneExpiredInvalidSnapshots(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		olderThan = 7 * 24 * time.Hour
	}
	tag, err := s.db.Exec(ctx, `
		DELETE FROM price_snapshots
		WHERE invalid = true
		  AND invalid_at IS NOT NULL
		  AND invalid_at < now() - make_interval(secs => $1)
	`, int64(olderThan.Seconds()))
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s Store) PreviousSnapshot(ctx context.Context, ruleID int64, modelName string) (PriceSnapshot, error) {
	var snapshot PriceSnapshot
	var groupRatio, inputPrice, outputPrice, cacheReadPrice, cacheWritePrice, requestPrice, upstreamBalance, rechargeMultiplier sql.NullFloat64
	var invalidAt sql.NullTime
	err := s.db.QueryRow(ctx, `
		SELECT p.id, p.rule_id, COALESCE(p.source_type, 'newapi'), COALESCE(p.site_id, 0), p.sub2api_upstream_id,
		       CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.name, '') ELSE COALESCE(s.name, '') END AS site_name,
		       COALESCE(NULLIF(p.source_base_url, ''), CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.base_url, '') ELSE COALESCE(s.base_url, '') END) AS site_base_url,
		       COALESCE(p.source_account, '') AS source_account,
		       p.category, COALESCE(c.name, p.category) AS category_name, p.model_keyword, p.model_name,
		       p.group_name, p.group_desc, p.quota_type, p.group_ratio, p.input_price, p.output_price,
		       p.cache_read_price, p.cache_write_price, p.request_price, p.upstream_balance, p.balance_unit,
		       p.online_topup_enabled, p.recharge_multiplier,
		       p.invalid, p.invalid_reason, p.invalid_at, p.raw, p.created_at
		FROM price_snapshots p
		LEFT JOIN sites s ON s.id = p.site_id
		LEFT JOIN sub2api_upstreams u ON u.id = p.sub2api_upstream_id
		LEFT JOIN categories c ON c.slug = p.category
		WHERE p.rule_id = $1 AND p.model_name = $2
		ORDER BY p.created_at DESC, p.id DESC
		LIMIT 1
	`, ruleID, strings.TrimSpace(modelName)).Scan(
		&snapshot.ID, &snapshot.RuleID, &snapshot.SourceType, &snapshot.SiteID, &snapshot.Sub2APIUpstreamID, &snapshot.SiteName, &snapshot.SiteBaseURL, &snapshot.SourceAccount,
		&snapshot.Category, &snapshot.CategoryName, &snapshot.ModelKeyword, &snapshot.ModelName,
		&snapshot.GroupName, &snapshot.GroupDesc, &snapshot.QuotaType, &groupRatio, &inputPrice,
		&outputPrice, &cacheReadPrice, &cacheWritePrice, &requestPrice, &upstreamBalance, &snapshot.BalanceUnit,
		&snapshot.OnlineTopupEnabled, &rechargeMultiplier,
		&snapshot.Invalid, &snapshot.InvalidReason, &invalidAt, &snapshot.Raw, &snapshot.CreatedAt,
	)
	if err != nil {
		return PriceSnapshot{}, err
	}
	snapshot.GroupRatio = floatPtr(groupRatio)
	snapshot.InputPrice = floatPtr(inputPrice)
	snapshot.OutputPrice = floatPtr(outputPrice)
	snapshot.CacheReadPrice = floatPtr(cacheReadPrice)
	snapshot.CacheWritePrice = floatPtr(cacheWritePrice)
	snapshot.RequestPrice = floatPtr(requestPrice)
	snapshot.UpstreamBalance = floatPtr(upstreamBalance)
	snapshot.RechargeMultiplier = floatPtr(rechargeMultiplier)
	snapshot.InvalidAt = timePtr(invalidAt)
	return snapshot, nil
}

func (s Store) CheapestLatestSnapshot(ctx context.Context, category string, modelName string, expectedCacheHitRatio float64) (PriceSnapshot, error) {
	hitRatio := normalizeExpectedCacheHitRatio(expectedCacheHitRatio)
	var snapshot PriceSnapshot
	var groupRatio, inputPrice, outputPrice, cacheReadPrice, cacheWritePrice, requestPrice, upstreamBalance, rechargeMultiplier sql.NullFloat64
	var invalidAt sql.NullTime
	err := s.db.QueryRow(ctx, `
		WITH latest AS (
			SELECT DISTINCT ON (p.rule_id)
			       p.id, p.rule_id, COALESCE(p.source_type, 'newapi') AS source_type, COALESCE(p.site_id, 0) AS site_id, p.sub2api_upstream_id,
			       CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.name, '') ELSE COALESCE(st.name, '') END AS site_name,
			       COALESCE(NULLIF(p.source_base_url, ''), CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.base_url, '') ELSE COALESCE(st.base_url, '') END) AS site_base_url,
			       COALESCE(p.source_account, '') AS source_account,
		       r.category, COALESCE(c.name, r.category) AS category_name, p.model_keyword, p.model_name,
		       p.group_name, p.group_desc, p.quota_type, p.group_ratio, p.input_price, p.output_price,
		       p.cache_read_price, p.cache_write_price, p.request_price, p.upstream_balance, p.balance_unit,
		       p.online_topup_enabled, p.recharge_multiplier,
		       p.invalid, p.invalid_reason, p.invalid_at, p.raw, p.created_at
			FROM price_snapshots p
			JOIN monitor_rules r ON r.id = p.rule_id
			LEFT JOIN sites st ON st.id = p.site_id
			LEFT JOIN sub2api_upstreams u ON u.id = p.sub2api_upstream_id
			LEFT JOIN categories c ON c.slug = r.category
			WHERE r.enabled = true
			  AND r.category = $1
			  AND p.model_name = $2
			  AND lower(trim(p.model_name)) = lower(trim(r.model_keyword))
			  AND p.invalid = false
			  AND NOT EXISTS (
			    SELECT 1
			    FROM jsonb_array_elements_text(COALESCE(c.blocked_group_keywords, '[]'::jsonb)) AS blocked(keyword)
			    WHERE trim(blocked.keyword) <> ''
			      AND lower(COALESCE(p.group_name, '') || ' ' || COALESCE(p.group_desc, '')) LIKE '%' || lower(trim(blocked.keyword)) || '%'
			  )
			ORDER BY p.rule_id, p.created_at DESC, p.id DESC
		)
		SELECT id, rule_id, source_type, site_id, sub2api_upstream_id, site_name, site_base_url, source_account, category, category_name, model_keyword,
		       model_name, group_name, group_desc, quota_type,
		       group_ratio, input_price, output_price, cache_read_price, cache_write_price,
		       request_price, upstream_balance, balance_unit, online_topup_enabled, recharge_multiplier, invalid, invalid_reason, invalid_at, raw, created_at
		FROM latest
		ORDER BY `+priceComparisonExpr("$3")+` ASC,
		         COALESCE(output_price, 1e308) ASC,
		         group_ratio ASC NULLS LAST,
		         id DESC
		LIMIT 1
	`, normalizeCategorySlug(category), strings.TrimSpace(modelName), hitRatio).Scan(
		&snapshot.ID, &snapshot.RuleID, &snapshot.SourceType, &snapshot.SiteID, &snapshot.Sub2APIUpstreamID, &snapshot.SiteName, &snapshot.SiteBaseURL, &snapshot.SourceAccount,
		&snapshot.Category, &snapshot.CategoryName, &snapshot.ModelKeyword, &snapshot.ModelName,
		&snapshot.GroupName, &snapshot.GroupDesc, &snapshot.QuotaType, &groupRatio, &inputPrice,
		&outputPrice, &cacheReadPrice, &cacheWritePrice, &requestPrice, &upstreamBalance, &snapshot.BalanceUnit,
		&snapshot.OnlineTopupEnabled, &rechargeMultiplier,
		&snapshot.Invalid, &snapshot.InvalidReason, &invalidAt, &snapshot.Raw, &snapshot.CreatedAt,
	)
	if err != nil {
		return PriceSnapshot{}, err
	}
	snapshot.GroupRatio = floatPtr(groupRatio)
	snapshot.InputPrice = floatPtr(inputPrice)
	snapshot.OutputPrice = floatPtr(outputPrice)
	snapshot.CacheReadPrice = floatPtr(cacheReadPrice)
	snapshot.CacheWritePrice = floatPtr(cacheWritePrice)
	snapshot.RequestPrice = floatPtr(requestPrice)
	snapshot.UpstreamBalance = floatPtr(upstreamBalance)
	snapshot.RechargeMultiplier = floatPtr(rechargeMultiplier)
	snapshot.InvalidAt = timePtr(invalidAt)
	return snapshot, nil
}

func (s Store) LatestSnapshots(ctx context.Context, limit int, category string, expectedCacheHitRatio float64) ([]PriceSnapshot, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	hitRatio := normalizeExpectedCacheHitRatio(expectedCacheHitRatio)
	category = normalizeCategorySlug(category)
	categoryFilter := ""
	args := []any{}
	if category != "" && category != "all" {
		args = append(args, category)
		categoryFilter = fmt.Sprintf("AND r.category = $%d", len(args))
	}
	args = append(args, limit)
	limitPlaceholder := fmt.Sprintf("$%d", len(args))
	rows, err := s.db.Query(ctx, `
		WITH filtered AS (
			SELECT p.*, r.category AS rule_category
			FROM price_snapshots p
			JOIN monitor_rules r ON r.id = p.rule_id
			WHERE lower(trim(p.model_name)) = lower(trim(r.model_keyword))
			`+categoryFilter+`
		),
		latest AS (
			SELECT DISTINCT ON (COALESCE(p.source_type, 'newapi'), lower(regexp_replace(trim(p.source_base_url), '/+$', '')), lower(trim(p.source_account)), p.rule_category, p.model_name, lower(trim(p.group_name)))
			       p.id, p.rule_id, COALESCE(p.source_type, 'newapi') AS source_type, COALESCE(p.site_id, 0) AS site_id, p.sub2api_upstream_id,
			       CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.name, '') ELSE COALESCE(s.name, '') END AS site_name,
			       COALESCE(NULLIF(p.source_base_url, ''), CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.base_url, '') ELSE COALESCE(s.base_url, '') END) AS site_base_url,
			       COALESCE(p.source_account, '') AS source_account,
			       p.rule_category AS category, COALESCE(c.name, p.rule_category) AS category_name, p.model_keyword, p.model_name, p.group_name,
			       p.group_desc, p.quota_type, p.group_ratio, p.input_price, p.output_price,
			       p.cache_read_price, p.cache_write_price, p.request_price, p.upstream_balance, p.balance_unit,
			       p.online_topup_enabled, p.recharge_multiplier,
			       p.invalid, p.invalid_reason, p.invalid_at, p.raw, p.created_at
			FROM filtered p
			LEFT JOIN sites s ON s.id = p.site_id
			LEFT JOIN sub2api_upstreams u ON u.id = p.sub2api_upstream_id
			LEFT JOIN categories c ON c.slug = p.rule_category
			ORDER BY COALESCE(p.source_type, 'newapi'), lower(regexp_replace(trim(p.source_base_url), '/+$', '')), lower(trim(p.source_account)), p.rule_category, p.model_name, lower(trim(p.group_name)), p.created_at DESC, p.id DESC
		)
		SELECT id, rule_id, source_type, site_id, sub2api_upstream_id, site_name, site_base_url, source_account, category, category_name, model_keyword,
		       model_name, group_name, group_desc, quota_type,
		       group_ratio, input_price, output_price, cache_read_price, cache_write_price,
		       request_price, upstream_balance, balance_unit, online_topup_enabled, recharge_multiplier, invalid, invalid_reason, invalid_at, raw, created_at
		FROM latest
		ORDER BY category,
		         invalid ASC,
		         model_name,
		         `+priceComparisonExpr(fmt.Sprintf("$%d", len(args)+1))+` ASC,
		         COALESCE(output_price, 1e308) ASC,
		         group_ratio ASC NULLS LAST,
		         group_name
		LIMIT `+limitPlaceholder+`
	`, append(args, hitRatio)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []PriceSnapshot
	for rows.Next() {
		var snapshot PriceSnapshot
		var groupRatio, inputPrice, outputPrice, cacheReadPrice, cacheWritePrice, requestPrice, upstreamBalance, rechargeMultiplier sql.NullFloat64
		var invalidAt sql.NullTime
		if err := rows.Scan(
			&snapshot.ID, &snapshot.RuleID, &snapshot.SourceType, &snapshot.SiteID, &snapshot.Sub2APIUpstreamID, &snapshot.SiteName, &snapshot.SiteBaseURL, &snapshot.SourceAccount,
			&snapshot.Category, &snapshot.CategoryName, &snapshot.ModelKeyword, &snapshot.ModelName,
			&snapshot.GroupName, &snapshot.GroupDesc, &snapshot.QuotaType, &groupRatio, &inputPrice,
			&outputPrice, &cacheReadPrice, &cacheWritePrice, &requestPrice, &upstreamBalance, &snapshot.BalanceUnit,
			&snapshot.OnlineTopupEnabled, &rechargeMultiplier,
			&snapshot.Invalid, &snapshot.InvalidReason, &invalidAt, &snapshot.Raw, &snapshot.CreatedAt,
		); err != nil {
			return nil, err
		}
		snapshot.GroupRatio = floatPtr(groupRatio)
		snapshot.InputPrice = floatPtr(inputPrice)
		snapshot.OutputPrice = floatPtr(outputPrice)
		snapshot.CacheReadPrice = floatPtr(cacheReadPrice)
		snapshot.CacheWritePrice = floatPtr(cacheWritePrice)
		snapshot.RequestPrice = floatPtr(requestPrice)
		snapshot.UpstreamBalance = floatPtr(upstreamBalance)
		snapshot.RechargeMultiplier = floatPtr(rechargeMultiplier)
		snapshot.InvalidAt = timePtr(invalidAt)
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

func (s Store) CheapestSyncCandidate(ctx context.Context, category string, modelName string, expectedCacheHitRatio float64, balanceThreshold float64) (PriceSnapshot, []PriceSnapshot, error) {
	candidates, skipped, err := s.SyncCandidates(ctx, category, modelName, expectedCacheHitRatio, balanceThreshold)
	if err != nil {
		return PriceSnapshot{}, nil, err
	}
	if len(candidates) == 0 {
		return PriceSnapshot{}, skipped, pgx.ErrNoRows
	}
	return candidates[0], skipped, nil
}

func (s Store) SyncCandidates(ctx context.Context, category string, modelName string, expectedCacheHitRatio float64, balanceThreshold float64) ([]PriceSnapshot, []PriceSnapshot, error) {
	snapshots, err := s.latestSnapshotsForModel(ctx, category, modelName, expectedCacheHitRatio)
	if err != nil {
		return nil, nil, err
	}
	candidates := make([]PriceSnapshot, 0, len(snapshots))
	skipped := make([]PriceSnapshot, 0)
	for _, snapshot := range snapshots {
		if snapshotBalanceInsufficient(snapshot, balanceThreshold) {
			skipped = append(skipped, snapshot)
			continue
		}
		candidates = append(candidates, snapshot)
	}
	return candidates, skipped, nil
}

func (s Store) latestSnapshotsForModel(ctx context.Context, category string, modelName string, expectedCacheHitRatio float64) ([]PriceSnapshot, error) {
	hitRatio := normalizeExpectedCacheHitRatio(expectedCacheHitRatio)
	rows, err := s.db.Query(ctx, `
		WITH latest AS (
			SELECT DISTINCT ON (p.rule_id)
			       p.id, p.rule_id, COALESCE(p.source_type, 'newapi') AS source_type, COALESCE(p.site_id, 0) AS site_id, p.sub2api_upstream_id,
			       CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.name, '') ELSE COALESCE(st.name, '') END AS site_name,
			       COALESCE(NULLIF(p.source_base_url, ''), CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.base_url, '') ELSE COALESCE(st.base_url, '') END) AS site_base_url,
			       COALESCE(p.source_account, '') AS source_account,
			       r.category, COALESCE(c.name, r.category) AS category_name, p.model_keyword, p.model_name,
			       p.group_name, p.group_desc, p.quota_type, p.group_ratio, p.input_price, p.output_price,
			       p.cache_read_price, p.cache_write_price, p.request_price, p.upstream_balance, p.balance_unit,
			       p.online_topup_enabled, p.recharge_multiplier,
			       p.invalid, p.invalid_reason, p.invalid_at, p.raw, p.created_at
			FROM price_snapshots p
			JOIN monitor_rules r ON r.id = p.rule_id
			LEFT JOIN sites st ON st.id = p.site_id
			LEFT JOIN sub2api_upstreams u ON u.id = p.sub2api_upstream_id
			LEFT JOIN categories c ON c.slug = r.category
			WHERE r.enabled = true
			  AND r.category = $1
			  AND p.model_name = $2
			  AND lower(trim(p.model_name)) = lower(trim(r.model_keyword))
			  AND p.invalid = false
			  AND NOT EXISTS (
			    SELECT 1
			    FROM jsonb_array_elements_text(COALESCE(c.blocked_group_keywords, '[]'::jsonb)) AS blocked(keyword)
			    WHERE trim(blocked.keyword) <> ''
			      AND lower(COALESCE(p.group_name, '') || ' ' || COALESCE(p.group_desc, '')) LIKE '%' || lower(trim(blocked.keyword)) || '%'
			  )
			ORDER BY p.rule_id, p.created_at DESC, p.id DESC
		)
		SELECT id, rule_id, source_type, site_id, sub2api_upstream_id, site_name, site_base_url, source_account, category, category_name, model_keyword,
		       model_name, group_name, group_desc, quota_type,
		       group_ratio, input_price, output_price, cache_read_price, cache_write_price,
		       request_price, upstream_balance, balance_unit, online_topup_enabled, recharge_multiplier, invalid, invalid_reason, invalid_at, raw, created_at
		FROM latest
		ORDER BY `+priceComparisonExpr("$3")+` ASC,
		         COALESCE(output_price, 1e308) ASC,
		         group_ratio ASC NULLS LAST,
		         id DESC
	`, normalizeCategorySlug(category), strings.TrimSpace(modelName), hitRatio)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []PriceSnapshot
	for rows.Next() {
		var snapshot PriceSnapshot
		var groupRatio, inputPrice, outputPrice, cacheReadPrice, cacheWritePrice, requestPrice, upstreamBalance, rechargeMultiplier sql.NullFloat64
		var invalidAt sql.NullTime
		if err := rows.Scan(
			&snapshot.ID, &snapshot.RuleID, &snapshot.SourceType, &snapshot.SiteID, &snapshot.Sub2APIUpstreamID, &snapshot.SiteName, &snapshot.SiteBaseURL, &snapshot.SourceAccount,
			&snapshot.Category, &snapshot.CategoryName, &snapshot.ModelKeyword, &snapshot.ModelName,
			&snapshot.GroupName, &snapshot.GroupDesc, &snapshot.QuotaType, &groupRatio, &inputPrice,
			&outputPrice, &cacheReadPrice, &cacheWritePrice, &requestPrice, &upstreamBalance, &snapshot.BalanceUnit,
			&snapshot.OnlineTopupEnabled, &rechargeMultiplier,
			&snapshot.Invalid, &snapshot.InvalidReason, &invalidAt, &snapshot.Raw, &snapshot.CreatedAt,
		); err != nil {
			return nil, err
		}
		snapshot.GroupRatio = floatPtr(groupRatio)
		snapshot.InputPrice = floatPtr(inputPrice)
		snapshot.OutputPrice = floatPtr(outputPrice)
		snapshot.CacheReadPrice = floatPtr(cacheReadPrice)
		snapshot.CacheWritePrice = floatPtr(cacheWritePrice)
		snapshot.RequestPrice = floatPtr(requestPrice)
		snapshot.UpstreamBalance = floatPtr(upstreamBalance)
		snapshot.RechargeMultiplier = floatPtr(rechargeMultiplier)
		snapshot.InvalidAt = timePtr(invalidAt)
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

func snapshotBalanceInsufficient(snapshot PriceSnapshot, threshold float64) bool {
	threshold = normalizeUpstreamBalanceThreshold(threshold)
	return snapshot.UpstreamBalance != nil && *snapshot.UpstreamBalance <= threshold
}

func normalizeSyncThresholdRatios(ratios map[string]float64) map[string]float64 {
	normalized := map[string]float64{}
	for category, ratio := range ratios {
		category = normalizeCategorySlug(category)
		if category == "" || ratio <= 0 {
			continue
		}
		normalized[category] = ratio
	}
	return normalized
}

func syncThresholdRatioForCategory(settings IntegrationSettings, category string) *float64 {
	category = normalizeCategorySlug(category)
	if category != "" && settings.SyncThresholdRatios != nil {
		if ratio, ok := settings.SyncThresholdRatios[category]; ok && ratio > 0 {
			return ptr(ratio)
		}
	}
	return settings.SyncThresholdRatio
}

func normalizeExpectedCacheHitRatio(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func normalizeUpstreamBalanceThreshold(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}

const (
	sub2APISyncAccountModeSchedulableOnly = "schedulable_only"
	sub2APISyncAccountModeDisableStatus   = "disable_status"
)

func normalizeSub2APISyncAccountMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case sub2APISyncAccountModeDisableStatus:
		return sub2APISyncAccountModeDisableStatus
	default:
		return sub2APISyncAccountModeSchedulableOnly
	}
}

func (s Store) GetIntegrationSettings(ctx context.Context) (IntegrationSettings, error) {
	var settings IntegrationSettings
	var syncThresholdRatio sql.NullFloat64
	var syncThresholdRatiosRaw []byte
	var templateConfigsRaw []byte
	err := s.db.QueryRow(ctx, `
		SELECT sub2api_enabled, sub2api_main_base_url, sub2api_admin_key,
		       sub2api_base_url, sub2api_access_token, sub2api_email, sub2api_password,
		       sub2api_sync_account_mode,
		       monitor_interval_minutes, monitor_rule_delay_seconds,
		       expected_cache_hit_ratio, upstream_balance_threshold,
		       sync_threshold_ratio, sync_threshold_ratios,
		       email_notify_enabled, email_notify_price_change, email_notify_sync_update,
		       smtp_host, smtp_port, smtp_encryption, smtp_username, smtp_password, smtp_from, smtp_to,
		       email_template_enabled, email_template_subject, email_template_body, email_template_configs,
		       updated_at
		FROM integration_settings
		WHERE id = true
	`).Scan(
		&settings.Sub2APIEnabled, &settings.Sub2APIMainBaseURL, &settings.Sub2APIAdminKey,
		&settings.Sub2APIBaseURL, &settings.Sub2APIAccessToken,
		&settings.Sub2APIEmail, &settings.Sub2APIPassword,
		&settings.Sub2APISyncAccountMode,
		&settings.MonitorIntervalMinutes, &settings.MonitorRuleDelaySeconds,
		&settings.ExpectedCacheHitRatio, &settings.UpstreamBalanceThreshold,
		&syncThresholdRatio, &syncThresholdRatiosRaw,
		&settings.EmailNotifyEnabled, &settings.EmailNotifyPriceChange, &settings.EmailNotifySyncUpdate,
		&settings.SMTPHost, &settings.SMTPPort, &settings.SMTPEncryption, &settings.SMTPUsername, &settings.SMTPPassword,
		&settings.SMTPFrom, &settings.SMTPTo,
		&settings.EmailTemplateEnabled, &settings.EmailTemplateSubject, &settings.EmailTemplateBody, &templateConfigsRaw,
		&settings.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return IntegrationSettings{Sub2APISyncAccountMode: sub2APISyncAccountModeSchedulableOnly}, nil
	}
	settings.EmailTemplateConfigs = decodeEmailTemplateConfigs(templateConfigsRaw)
	settings.SyncThresholdRatio = floatPtr(syncThresholdRatio)
	settings.SyncThresholdRatios = decodeSyncThresholdRatios(syncThresholdRatiosRaw)
	if settings.Sub2APIMainBaseURL == "" {
		settings.Sub2APIMainBaseURL = settings.Sub2APIBaseURL
	}
	if settings.Sub2APIAdminKey == "" {
		settings.Sub2APIAdminKey = settings.Sub2APIAccessToken
	}
	settings.SMTPEncryption = normalizeSMTPEncryption(settings.SMTPEncryption)
	settings.Sub2APISyncAccountMode = normalizeSub2APISyncAccountMode(settings.Sub2APISyncAccountMode)
	settings.MonitorIntervalMinutes, settings.MonitorRuleDelaySeconds = normalizeMonitorScheduleSettings(settings.MonitorIntervalMinutes, settings.MonitorRuleDelaySeconds)
	settings.ExpectedCacheHitRatio = normalizeExpectedCacheHitRatio(settings.ExpectedCacheHitRatio)
	settings.UpstreamBalanceThreshold = normalizeUpstreamBalanceThreshold(settings.UpstreamBalanceThreshold)
	settings.Sub2APIBaseURL = settings.Sub2APIMainBaseURL
	settings.Sub2APIAccessToken = settings.Sub2APIAdminKey
	return settings, err
}

func (s Store) SaveIntegrationSettings(ctx context.Context, input SettingsInput) (IntegrationSettings, error) {
	input.Sub2APIMainBaseURL = normalizeBaseURL(firstNonEmpty(input.Sub2APIMainBaseURL, input.Sub2APIBaseURL))
	input.Sub2APIAdminKey = strings.TrimSpace(firstNonEmpty(input.Sub2APIAdminKey, input.Sub2APIAccessToken))
	input.Sub2APIEmail = strings.TrimSpace(input.Sub2APIEmail)
	input.Sub2APISyncAccountMode = normalizeSub2APISyncAccountMode(input.Sub2APISyncAccountMode)
	input.SMTPHost = strings.TrimSpace(input.SMTPHost)
	input.SMTPEncryption = normalizeSMTPEncryption(input.SMTPEncryption)
	input.SMTPUsername = strings.TrimSpace(input.SMTPUsername)
	input.SMTPFrom = strings.TrimSpace(input.SMTPFrom)
	input.SMTPTo = strings.TrimSpace(input.SMTPTo)
	input.EmailTemplateSubject = strings.TrimSpace(input.EmailTemplateSubject)
	input.EmailTemplateConfigs = normalizeEmailTemplateConfigs(input.EmailTemplateConfigs)
	input.SyncThresholdRatios = normalizeSyncThresholdRatios(input.SyncThresholdRatios)
	input.MonitorIntervalMinutes, input.MonitorRuleDelaySeconds = normalizeMonitorScheduleSettings(input.MonitorIntervalMinutes, input.MonitorRuleDelaySeconds)
	input.ExpectedCacheHitRatio = normalizeExpectedCacheHitRatio(input.ExpectedCacheHitRatio)
	input.UpstreamBalanceThreshold = normalizeUpstreamBalanceThreshold(input.UpstreamBalanceThreshold)
	if input.SMTPPort <= 0 {
		input.SMTPPort = 587
	}
	if input.SyncThresholdRatio < 0 {
		input.SyncThresholdRatio = 0
	}

	var existing IntegrationSettings
	_ = s.db.QueryRow(ctx, `
		SELECT sub2api_main_base_url, sub2api_admin_key, sub2api_access_token, sub2api_password,
		       smtp_password
		FROM integration_settings
		WHERE id = true
	`).Scan(
		&existing.Sub2APIMainBaseURL, &existing.Sub2APIAdminKey, &existing.Sub2APIAccessToken,
		&existing.Sub2APIPassword, &existing.SMTPPassword,
	)
	if input.Sub2APIMainBaseURL == "" {
		input.Sub2APIMainBaseURL = existing.Sub2APIMainBaseURL
	}
	if input.Sub2APIAdminKey == "" {
		input.Sub2APIAdminKey = firstNonEmpty(existing.Sub2APIAdminKey, existing.Sub2APIAccessToken)
	}
	if input.Sub2APIPassword == "" {
		input.Sub2APIPassword = existing.Sub2APIPassword
	}
	if input.SMTPPassword == "" {
		input.SMTPPassword = existing.SMTPPassword
	}
	if input.Sub2APIEnabled && input.Sub2APIMainBaseURL == "" {
		return IntegrationSettings{}, fmt.Errorf("sub2api main base url is required")
	}
	if input.Sub2APIEnabled && input.Sub2APIAdminKey == "" {
		return IntegrationSettings{}, fmt.Errorf("sub2api admin key is required")
	}
	if input.EmailNotifyEnabled {
		if input.SMTPHost == "" || input.SMTPPort <= 0 || input.SMTPFrom == "" || input.SMTPTo == "" {
			return IntegrationSettings{}, fmt.Errorf("smtp host, port, from and recipients are required when email notification is enabled")
		}
	}

	var settings IntegrationSettings
	var syncThresholdRatio sql.NullFloat64
	var savedSyncThresholdRatiosRaw []byte
	templateConfigsRaw, err := json.Marshal(input.EmailTemplateConfigs)
	if err != nil {
		return IntegrationSettings{}, err
	}
	syncThresholdRatiosRaw, err := json.Marshal(input.SyncThresholdRatios)
	if err != nil {
		return IntegrationSettings{}, err
	}
	var savedTemplateConfigsRaw []byte
	err = s.db.QueryRow(ctx, `
		INSERT INTO integration_settings (
			id, sub2api_enabled, sub2api_main_base_url, sub2api_admin_key,
			sub2api_base_url, sub2api_access_token, sub2api_email, sub2api_password,
			sub2api_sync_account_mode,
			monitor_interval_minutes, monitor_rule_delay_seconds,
			expected_cache_hit_ratio, upstream_balance_threshold,
			sync_threshold_ratio, sync_threshold_ratios,
			email_notify_enabled, email_notify_price_change, email_notify_sync_update,
			smtp_host, smtp_port, smtp_encryption, smtp_username, smtp_password, smtp_from, smtp_to,
			email_template_enabled, email_template_subject, email_template_body, email_template_configs,
			updated_at
		)
		VALUES (true, $1, $2, $3, $2, $3, $4, $5, $6, $7, $8, $9, $10, CASE WHEN $11::double precision > 0 THEN $11::double precision ELSE NULL END, $12::jsonb, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26::jsonb, now())
		ON CONFLICT (id) DO UPDATE
		SET sub2api_enabled = EXCLUDED.sub2api_enabled,
		    sub2api_main_base_url = EXCLUDED.sub2api_main_base_url,
		    sub2api_admin_key = EXCLUDED.sub2api_admin_key,
		    sub2api_base_url = EXCLUDED.sub2api_base_url,
		    sub2api_access_token = EXCLUDED.sub2api_access_token,
		    sub2api_email = EXCLUDED.sub2api_email,
		    sub2api_password = EXCLUDED.sub2api_password,
		    sub2api_sync_account_mode = EXCLUDED.sub2api_sync_account_mode,
		    monitor_interval_minutes = EXCLUDED.monitor_interval_minutes,
		    monitor_rule_delay_seconds = EXCLUDED.monitor_rule_delay_seconds,
		    expected_cache_hit_ratio = EXCLUDED.expected_cache_hit_ratio,
		    upstream_balance_threshold = EXCLUDED.upstream_balance_threshold,
		    sync_threshold_ratio = EXCLUDED.sync_threshold_ratio,
		    sync_threshold_ratios = EXCLUDED.sync_threshold_ratios,
		    email_notify_enabled = EXCLUDED.email_notify_enabled,
		    email_notify_price_change = EXCLUDED.email_notify_price_change,
		    email_notify_sync_update = EXCLUDED.email_notify_sync_update,
		    smtp_host = EXCLUDED.smtp_host,
		    smtp_port = EXCLUDED.smtp_port,
		    smtp_encryption = EXCLUDED.smtp_encryption,
		    smtp_username = EXCLUDED.smtp_username,
		    smtp_password = EXCLUDED.smtp_password,
		    smtp_from = EXCLUDED.smtp_from,
		    smtp_to = EXCLUDED.smtp_to,
		    email_template_enabled = EXCLUDED.email_template_enabled,
		    email_template_subject = EXCLUDED.email_template_subject,
		    email_template_body = EXCLUDED.email_template_body,
		    email_template_configs = EXCLUDED.email_template_configs,
		    updated_at = now()
		RETURNING sub2api_enabled, sub2api_main_base_url, sub2api_admin_key,
		          sub2api_base_url, sub2api_access_token, sub2api_email, sub2api_password,
		          sub2api_sync_account_mode,
		          monitor_interval_minutes, monitor_rule_delay_seconds,
		          expected_cache_hit_ratio, upstream_balance_threshold,
		          sync_threshold_ratio, sync_threshold_ratios,
		          email_notify_enabled, email_notify_price_change, email_notify_sync_update,
		          smtp_host, smtp_port, smtp_encryption, smtp_username, smtp_password, smtp_from, smtp_to,
		          email_template_enabled, email_template_subject, email_template_body, email_template_configs,
		          updated_at
	`,
		input.Sub2APIEnabled, input.Sub2APIMainBaseURL, input.Sub2APIAdminKey, input.Sub2APIEmail, input.Sub2APIPassword, input.Sub2APISyncAccountMode,
		input.MonitorIntervalMinutes, input.MonitorRuleDelaySeconds,
		input.ExpectedCacheHitRatio, input.UpstreamBalanceThreshold,
		input.SyncThresholdRatio, string(syncThresholdRatiosRaw), input.EmailNotifyEnabled, input.EmailNotifyPriceChange, input.EmailNotifySyncUpdate,
		input.SMTPHost, input.SMTPPort, input.SMTPEncryption, input.SMTPUsername, input.SMTPPassword, input.SMTPFrom, input.SMTPTo,
		input.EmailTemplateEnabled, input.EmailTemplateSubject, input.EmailTemplateBody, string(templateConfigsRaw),
	).Scan(
		&settings.Sub2APIEnabled, &settings.Sub2APIMainBaseURL, &settings.Sub2APIAdminKey,
		&settings.Sub2APIBaseURL, &settings.Sub2APIAccessToken,
		&settings.Sub2APIEmail, &settings.Sub2APIPassword,
		&settings.Sub2APISyncAccountMode,
		&settings.MonitorIntervalMinutes, &settings.MonitorRuleDelaySeconds,
		&settings.ExpectedCacheHitRatio, &settings.UpstreamBalanceThreshold,
		&syncThresholdRatio, &savedSyncThresholdRatiosRaw,
		&settings.EmailNotifyEnabled, &settings.EmailNotifyPriceChange, &settings.EmailNotifySyncUpdate,
		&settings.SMTPHost, &settings.SMTPPort, &settings.SMTPEncryption, &settings.SMTPUsername, &settings.SMTPPassword,
		&settings.SMTPFrom, &settings.SMTPTo,
		&settings.EmailTemplateEnabled, &settings.EmailTemplateSubject, &settings.EmailTemplateBody, &savedTemplateConfigsRaw,
		&settings.UpdatedAt,
	)
	settings.EmailTemplateConfigs = decodeEmailTemplateConfigs(savedTemplateConfigsRaw)
	settings.SyncThresholdRatio = floatPtr(syncThresholdRatio)
	settings.SyncThresholdRatios = decodeSyncThresholdRatios(savedSyncThresholdRatiosRaw)
	settings.Sub2APISyncAccountMode = normalizeSub2APISyncAccountMode(settings.Sub2APISyncAccountMode)
	settings.ExpectedCacheHitRatio = normalizeExpectedCacheHitRatio(settings.ExpectedCacheHitRatio)
	settings.UpstreamBalanceThreshold = normalizeUpstreamBalanceThreshold(settings.UpstreamBalanceThreshold)
	settings.MonitorIntervalMinutes, settings.MonitorRuleDelaySeconds = normalizeMonitorScheduleSettings(settings.MonitorIntervalMinutes, settings.MonitorRuleDelaySeconds)
	return settings, err
}

func decodeSyncThresholdRatios(raw []byte) map[string]float64 {
	if len(raw) == 0 {
		return map[string]float64{}
	}
	var values map[string]float64
	if err := json.Unmarshal(raw, &values); err != nil {
		return map[string]float64{}
	}
	return normalizeSyncThresholdRatios(values)
}

func normalizeMonitorScheduleSettings(roundMinutes int, ruleDelaySeconds int) (int, int) {
	if roundMinutes <= 0 {
		roundMinutes = 15
	}
	if roundMinutes > 1440 {
		roundMinutes = 1440
	}
	if ruleDelaySeconds <= 0 {
		ruleDelaySeconds = 60
	}
	if ruleDelaySeconds > 3600 {
		ruleDelaySeconds = 3600
	}
	return roundMinutes, ruleDelaySeconds
}

func normalizeEmailTemplateConfigs(configs map[string]EmailTemplateConfig) map[string]EmailTemplateConfig {
	normalized := map[string]EmailTemplateConfig{}
	for key, config := range configs {
		key = normalizeEmailTemplateType(key)
		if key == "" {
			continue
		}
		config.Subject = strings.TrimSpace(config.Subject)
		if config.Subject == "" && strings.TrimSpace(config.Body) == "" {
			continue
		}
		normalized[key] = config
	}
	return normalized
}

func decodeEmailTemplateConfigs(raw []byte) map[string]EmailTemplateConfig {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" {
		return map[string]EmailTemplateConfig{}
	}
	var configs map[string]EmailTemplateConfig
	if err := json.Unmarshal(raw, &configs); err != nil {
		return map[string]EmailTemplateConfig{}
	}
	return normalizeEmailTemplateConfigs(configs)
}

func normalizeEmailTemplateType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "price_change", "sync_update", "sync_failure", "account_update", "low_balance_skip":
		return value
	default:
		return ""
	}
}

func (s Store) EnsureAdminCredentials(ctx context.Context, username string, passwordHash string) error {
	username = strings.TrimSpace(username)
	passwordHash = strings.TrimSpace(passwordHash)
	if username == "" || passwordHash == "" {
		return nil
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO admin_credentials (id, username, password_hash)
		VALUES (true, $1, $2)
		ON CONFLICT (id) DO NOTHING
	`, username, passwordHash)
	return err
}

func (s Store) GetAdminCredentials(ctx context.Context) (AdminCredentials, error) {
	var credentials AdminCredentials
	err := s.db.QueryRow(ctx, `
		SELECT username, password_hash, updated_at
		FROM admin_credentials
		WHERE id = true
	`).Scan(&credentials.Username, &credentials.PasswordHash, &credentials.UpdatedAt)
	if err != nil {
		return AdminCredentials{}, err
	}
	return credentials, nil
}

func (s Store) SaveAdminCredentials(ctx context.Context, username string, passwordHash string) (AdminCredentials, error) {
	username = strings.TrimSpace(username)
	passwordHash = strings.TrimSpace(passwordHash)
	if username == "" || passwordHash == "" {
		return AdminCredentials{}, fmt.Errorf("username and password are required")
	}
	var credentials AdminCredentials
	err := s.db.QueryRow(ctx, `
		INSERT INTO admin_credentials (id, username, password_hash)
		VALUES (true, $1, $2)
		ON CONFLICT (id) DO UPDATE
		SET username = EXCLUDED.username,
		    password_hash = EXCLUDED.password_hash,
		    updated_at = now()
		RETURNING username, password_hash, updated_at
	`, username, passwordHash).Scan(&credentials.Username, &credentials.PasswordHash, &credentials.UpdatedAt)
	return credentials, err
}

func notFound(err error) bool {
	return err == pgx.ErrNoRows
}

func floatPtr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

func floatValueFromPtr(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func timePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func normalizeCategorySlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if value == "claude" {
		return "claud"
	}
	var out strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			out.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ':
			if !lastDash && out.Len() > 0 {
				out.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(out.String(), "-")
}

func normalizeSMTPEncryption(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ssl", "tls", "smtps":
		return "ssl"
	case "starttls":
		return "starttls"
	case "plain", "none", "off":
		return "plain"
	default:
		return "auto"
	}
}

func normalizeSub2APIUpstreamInput(input Sub2APIUpstreamInput) Sub2APIUpstreamInput {
	input.Name = strings.TrimSpace(input.Name)
	input.BaseURL = normalizeBaseURL(input.BaseURL)
	input.Email = strings.TrimSpace(input.Email)
	input.Password = strings.TrimSpace(input.Password)
	input.AuthToken = strings.TrimSpace(input.AuthToken)
	input.TOTPCode = strings.TrimSpace(input.TOTPCode)
	return input
}

func normalizeSiteInput(input SiteInput) SiteInput {
	input.Name = strings.TrimSpace(input.Name)
	input.BaseURL = normalizeBaseURL(input.BaseURL)
	input.Username = strings.TrimSpace(input.Username)
	input.AccessToken = strings.TrimSpace(input.AccessToken)
	input.TOTPCode = strings.TrimSpace(input.TOTPCode)
	return input
}
