package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
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
	Name     string `json:"name"`
	BaseURL  string `json:"base_url"`
	Username string `json:"username"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code"`
}

type CategoryInput struct {
	Name                 string            `json:"name"`
	Slug                 string            `json:"slug"`
	Sub2APIMainGroupID   int64             `json:"sub2api_main_group_id"`
	Sub2APIMainGroupName string            `json:"sub2api_main_group_name"`
	Sub2APIMainGroups    []Sub2APIGroupRef `json:"sub2api_main_groups"`
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
}

type SettingsInput struct {
	Sub2APIEnabled         bool    `json:"sub2api_enabled"`
	Sub2APIMainBaseURL     string  `json:"sub2api_main_base_url"`
	Sub2APIAdminKey        string  `json:"sub2api_admin_key"`
	Sub2APIBaseURL         string  `json:"sub2api_base_url"`
	Sub2APIAccessToken     string  `json:"sub2api_access_token"`
	Sub2APIEmail           string  `json:"sub2api_email"`
	Sub2APIPassword        string  `json:"sub2api_password"`
	SyncThresholdRatio     float64 `json:"sync_threshold_ratio"`
	EmailNotifyEnabled     bool    `json:"email_notify_enabled"`
	EmailNotifyPriceChange bool    `json:"email_notify_price_change"`
	EmailNotifySyncUpdate  bool    `json:"email_notify_sync_update"`
	SMTPHost               string  `json:"smtp_host"`
	SMTPPort               int     `json:"smtp_port"`
	SMTPEncryption         string  `json:"smtp_encryption"`
	SMTPUsername           string  `json:"smtp_username"`
	SMTPPassword           string  `json:"smtp_password"`
	SMTPFrom               string  `json:"smtp_from"`
	SMTPTo                 string  `json:"smtp_to"`
}

type AdminCredentialInput struct {
	Username        string `json:"username"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s Store) CreateSite(ctx context.Context, input SiteInput) (Site, error) {
	input = normalizeSiteInput(input)
	if input.Name == "" || input.BaseURL == "" || input.Username == "" || input.Password == "" {
		return Site{}, fmt.Errorf("site name, base url, username and password are required")
	}
	if err := s.ensureUniqueSite(ctx, input, 0); err != nil {
		return Site{}, err
	}

	var site Site
	err := s.db.QueryRow(ctx, `
		INSERT INTO sites (name, base_url, username, password, totp_code)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, base_url, username, password, totp_code, user_id, access_token, last_error, last_run_at, created_at, updated_at
	`, input.Name, input.BaseURL, input.Username, input.Password, input.TOTPCode).Scan(
		&site.ID, &site.Name, &site.BaseURL, &site.Username, &site.Password, &site.TOTPCode,
		&site.UserID, &site.AccessToken, &site.LastError, &site.LastRunAt, &site.CreatedAt, &site.UpdatedAt,
	)
	return site, err
}

func (s Store) ensureUniqueSite(ctx context.Context, input SiteInput, excludeID int64) error {
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
		SELECT id, name, base_url, username, password, totp_code, user_id, access_token, last_error, last_run_at, created_at, updated_at
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
			&site.UserID, &site.AccessToken, &site.LastError, &site.LastRunAt, &site.CreatedAt, &site.UpdatedAt,
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
		SELECT id, name, base_url, username, password, totp_code, user_id, access_token, last_error, last_run_at, created_at, updated_at
		FROM sites
		WHERE id = $1
	`, siteID).Scan(
		&site.ID, &site.Name, &site.BaseURL, &site.Username, &site.Password, &site.TOTPCode,
		&site.UserID, &site.AccessToken, &site.LastError, &site.LastRunAt, &site.CreatedAt, &site.UpdatedAt,
	)
	return site, err
}

func (s Store) UpdateSite(ctx context.Context, siteID int64, input SiteInput) (Site, error) {
	input = normalizeSiteInput(input)
	if siteID <= 0 || input.Name == "" || input.BaseURL == "" || input.Username == "" {
		return Site{}, fmt.Errorf("site id, name, base url and username are required")
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
		    access_token = CASE WHEN base_url <> $3 OR username <> $4 OR ($5 <> '' AND password <> $5) THEN '' ELSE access_token END,
		    last_error = '',
		    updated_at = now()
		WHERE id = $1
		RETURNING id, name, base_url, username, password, totp_code, user_id, access_token, last_error, last_run_at, created_at, updated_at
	`, siteID, input.Name, input.BaseURL, input.Username, input.Password, input.TOTPCode).Scan(
		&site.ID, &site.Name, &site.BaseURL, &site.Username, &site.Password, &site.TOTPCode,
		&site.UserID, &site.AccessToken, &site.LastError, &site.LastRunAt, &site.CreatedAt, &site.UpdatedAt,
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
	_, err := s.db.Exec(ctx, `
		UPDATE sites
		SET user_id = $2, access_token = $3, last_run_at = $4, last_error = $5, updated_at = now()
		WHERE id = $1
	`, siteID, userID, token, runAt, lastErr)
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
		RETURNING id, name, base_url, email, password, auth_token, totp_code, last_error, last_check_at, created_at, updated_at
	`, input.Name, input.BaseURL, input.Email, input.Password, input.AuthToken, input.TOTPCode).Scan(
		&upstream.ID, &upstream.Name, &upstream.BaseURL, &upstream.Email, &upstream.Password, &upstream.AuthToken,
		&upstream.TOTPCode, &upstream.LastError, &upstream.LastCheckAt, &upstream.CreatedAt, &upstream.UpdatedAt,
	)
	return upstream, err
}

func (s Store) ListSub2APIUpstreams(ctx context.Context) ([]Sub2APIUpstream, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, base_url, email, password, auth_token, totp_code, last_error, last_check_at, created_at, updated_at
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
			&upstream.TOTPCode, &upstream.LastError, &upstream.LastCheckAt, &upstream.CreatedAt, &upstream.UpdatedAt,
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
		SELECT id, name, base_url, email, password, auth_token, totp_code, last_error, last_check_at, created_at, updated_at
		FROM sub2api_upstreams
		WHERE id = $1
	`, upstreamID).Scan(
		&upstream.ID, &upstream.Name, &upstream.BaseURL, &upstream.Email, &upstream.Password, &upstream.AuthToken,
		&upstream.TOTPCode, &upstream.LastError, &upstream.LastCheckAt, &upstream.CreatedAt, &upstream.UpdatedAt,
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
		    last_error = '',
		    updated_at = now()
		WHERE id = $1
		RETURNING id, name, base_url, email, password, auth_token, totp_code, last_error, last_check_at, created_at, updated_at
	`, upstreamID, input.Name, input.BaseURL, input.Email, input.Password, input.AuthToken, input.TOTPCode).Scan(
		&upstream.ID, &upstream.Name, &upstream.BaseURL, &upstream.Email, &upstream.Password, &upstream.AuthToken,
		&upstream.TOTPCode, &upstream.LastError, &upstream.LastCheckAt, &upstream.CreatedAt, &upstream.UpdatedAt,
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
	_, err := s.db.Exec(ctx, `
		UPDATE sub2api_upstreams
		SET last_check_at = $2, last_error = $3, updated_at = now()
		WHERE id = $1
	`, upstreamID, checkedAt, lastErr)
	return err
}

func (s Store) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, slug, sub2api_main_group_id, sub2api_main_group_name, COALESCE(sub2api_main_groups, '[]'::jsonb), created_at, updated_at
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
		if err := rows.Scan(
			&category.ID, &category.Name, &category.Slug, &category.Sub2APIMainGroupID, &category.Sub2APIMainGroupName,
			&groupsRaw,
			&category.CreatedAt, &category.UpdatedAt,
		); err != nil {
			return nil, err
		}
		category.Sub2APIMainGroups = normalizeCategoryGroupRefs(category.Sub2APIMainGroupID, category.Sub2APIMainGroupName, groupsRaw)
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
	primary := primaryCategoryGroup(groups)
	if slug == "" {
		slug = normalizeCategorySlug(name)
	}
	if name == "" || slug == "" {
		return Category{}, fmt.Errorf("category name is required")
	}

	var category Category
	err = s.db.QueryRow(ctx, `
		INSERT INTO categories (name, slug, sub2api_main_group_id, sub2api_main_group_name, sub2api_main_groups)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		ON CONFLICT (slug) DO UPDATE
		SET name = EXCLUDED.name,
		    sub2api_main_group_id = EXCLUDED.sub2api_main_group_id,
		    sub2api_main_group_name = EXCLUDED.sub2api_main_group_name,
		    sub2api_main_groups = EXCLUDED.sub2api_main_groups,
		    updated_at = now()
		RETURNING id, name, slug, sub2api_main_group_id, sub2api_main_group_name, COALESCE(sub2api_main_groups, '[]'::jsonb), created_at, updated_at
	`, name, slug, primary.ID, primary.Name, string(groupsJSON)).Scan(
		&category.ID, &category.Name, &category.Slug, &category.Sub2APIMainGroupID, &category.Sub2APIMainGroupName,
		&groupsJSON,
		&category.CreatedAt, &category.UpdatedAt,
	)
	category.Sub2APIMainGroups = normalizeCategoryGroupRefs(category.Sub2APIMainGroupID, category.Sub2APIMainGroupName, groupsJSON)
	return category, err
}

func (s Store) UpdateCategory(ctx context.Context, categoryID int64, input CategoryInput) (Category, error) {
	name := strings.TrimSpace(input.Name)
	slug := normalizeCategorySlug(input.Slug)
	groups, groupsJSON, err := normalizeCategoryInputGroups(input)
	if err != nil {
		return Category{}, err
	}
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
		    updated_at = now()
		WHERE id = $1
		RETURNING id, name, slug, sub2api_main_group_id, sub2api_main_group_name, COALESCE(sub2api_main_groups, '[]'::jsonb), created_at, updated_at
	`, categoryID, name, slug, primary.ID, primary.Name, string(groupsJSON)).Scan(
		&category.ID, &category.Name, &category.Slug, &category.Sub2APIMainGroupID, &category.Sub2APIMainGroupName,
		&groupsJSON,
		&category.CreatedAt, &category.UpdatedAt,
	); err != nil {
		return Category{}, err
	}
	category.Sub2APIMainGroups = normalizeCategoryGroupRefs(category.Sub2APIMainGroupID, category.Sub2APIMainGroupName, groupsJSON)
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
	err := s.db.QueryRow(ctx, `
		SELECT id, name, slug, sub2api_main_group_id, sub2api_main_group_name, COALESCE(sub2api_main_groups, '[]'::jsonb), created_at, updated_at
		FROM categories
		WHERE slug = $1
	`, slug).Scan(
		&category.ID, &category.Name, &category.Slug, &category.Sub2APIMainGroupID, &category.Sub2APIMainGroupName,
		&groupsRaw,
		&category.CreatedAt, &category.UpdatedAt,
	)
	category.Sub2APIMainGroups = normalizeCategoryGroupRefs(category.Sub2APIMainGroupID, category.Sub2APIMainGroupName, groupsRaw)
	return category, err
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
			VALUES ($1, NULLIF($2, 0), $3, $4, $5, $6, $7, true, $8, $9::int, CASE WHEN $8 THEN now() + make_interval(mins => $9::int) ELSE NULL END, $10, $11, NULLIF($12, 0), $13, $14)
			RETURNING id, source_type, COALESCE(site_id, 0) AS site_id, sub2api_upstream_id, category, model_keyword, model_name, group_name, enabled,
			          schedule_enabled, interval_minutes, next_run_at, last_scheduled_run_at,
			          sync_enabled, sync_base_group, sync_threshold_ratio, sub2api_group_name, sub2api_group_id,
			          last_sync_at, sync_status, sync_error, created_at, updated_at
		)
		SELECT r.id, r.source_type, r.site_id, COALESCE(s.name, ''), r.sub2api_upstream_id, COALESCE(u.name, ''),
		       CASE WHEN r.source_type = 'sub2api' THEN COALESCE(u.name, '') ELSE COALESCE(s.name, '') END AS source_name,
		       CASE WHEN r.source_type = 'sub2api' THEN COALESCE(u.base_url, '') ELSE COALESCE(s.base_url, '') END AS source_base_url,
		       r.category, COALESCE(c.name, r.category),
		       r.model_keyword, r.model_name, COALESCE(r.group_name, ''), r.enabled,
		       r.schedule_enabled, r.interval_minutes, r.next_run_at, r.last_scheduled_run_at,
		       r.sync_enabled, r.sync_base_group, r.sync_threshold_ratio, r.sub2api_group_name, r.sub2api_group_id,
		       r.last_sync_at, r.sync_status, r.sync_error, r.created_at, r.updated_at
		FROM inserted r
		LEFT JOIN sites s ON s.id = r.site_id
		LEFT JOIN sub2api_upstreams u ON u.id = r.sub2api_upstream_id
		LEFT JOIN categories c ON c.slug = r.category
	`, input.SourceType, input.SiteID, input.Sub2APIUpstreamID, input.Category, input.ModelKeyword, input.ModelName, input.GroupName,
		input.ScheduleEnabled, input.IntervalMinutes, input.SyncEnabled, input.SyncBaseGroup,
		input.SyncThresholdRatio, input.Sub2APIGroupName, input.Sub2APIGroupID).Scan(
		&rule.ID, &rule.SourceType, &rule.SiteID, &rule.SiteName, &rule.Sub2APIUpstreamID, &rule.Sub2APIUpstreamName,
		&rule.SourceName, &rule.SourceBaseURL, &rule.Category, &rule.CategoryName,
		&rule.ModelKeyword, &rule.ModelName, &rule.GroupName, &rule.Enabled,
		&rule.ScheduleEnabled, &rule.IntervalMinutes, &rule.NextRunAt, &rule.LastScheduledRunAt,
		&rule.SyncEnabled, &rule.SyncBaseGroup, &rule.SyncThresholdRatio, &rule.Sub2APIGroupName, &rule.Sub2APIGroupID,
		&rule.LastSyncAt, &rule.SyncStatus, &rule.SyncError, &rule.CreatedAt, &rule.UpdatedAt,
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
		       r.category, COALESCE(c.name, r.category),
		       r.model_keyword, r.model_name, COALESCE(r.group_name, ''), r.enabled,
		       r.schedule_enabled, r.interval_minutes, r.next_run_at, r.last_scheduled_run_at,
		       r.sync_enabled, r.sync_base_group, r.sync_threshold_ratio, r.sub2api_group_name, r.sub2api_group_id,
		       r.last_sync_at, r.sync_status, r.sync_error, r.created_at, r.updated_at
		FROM updated r
		LEFT JOIN sites s ON s.id = r.site_id
		LEFT JOIN sub2api_upstreams u ON u.id = r.sub2api_upstream_id
		LEFT JOIN categories c ON c.slug = r.category
	`, ruleID, input.SourceType, input.SiteID, input.Sub2APIUpstreamID, input.Category, input.ModelKeyword, input.ModelName, input.GroupName, input.Enabled,
		input.ScheduleEnabled, input.IntervalMinutes, input.SyncEnabled, input.SyncBaseGroup,
		input.SyncThresholdRatio, input.Sub2APIGroupName, input.Sub2APIGroupID).Scan(
		&rule.ID, &rule.SourceType, &rule.SiteID, &rule.SiteName, &rule.Sub2APIUpstreamID, &rule.Sub2APIUpstreamName,
		&rule.SourceName, &rule.SourceBaseURL, &rule.Category, &rule.CategoryName,
		&rule.ModelKeyword, &rule.ModelName, &rule.GroupName, &rule.Enabled,
		&rule.ScheduleEnabled, &rule.IntervalMinutes, &rule.NextRunAt, &rule.LastScheduledRunAt,
		&rule.SyncEnabled, &rule.SyncBaseGroup, &rule.SyncThresholdRatio, &rule.Sub2APIGroupName, &rule.Sub2APIGroupID,
		&rule.LastSyncAt, &rule.SyncStatus, &rule.SyncError, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		return Rule{}, err
	}
	if err := s.syncRuleSnapshotsCategory(ctx, rule.ID, rule.Category); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

func (s Store) ensureUniqueRule(ctx context.Context, input RuleInput, excludeID int64) error {
	var existingID int64
	err := s.db.QueryRow(ctx, `
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
	if err == nil {
		return fmt.Errorf("相同站点、分类和模型的监控规则已存在，请勿重复添加")
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
		           PARTITION BY COALESCE(source_type, 'newapi'), COALESCE(site_id, 0), sub2api_upstream_id,
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
		       r.category, COALESCE(c.name, r.category),
		       r.model_keyword, r.model_name, COALESCE(r.group_name, ''), r.enabled,
		       r.schedule_enabled, r.interval_minutes, r.next_run_at, r.last_scheduled_run_at,
		       r.sync_enabled, r.sync_base_group, r.sync_threshold_ratio, r.sub2api_group_name, r.sub2api_group_id,
		       r.last_sync_at, r.sync_status, r.sync_error, r.created_at, r.updated_at
		FROM monitor_rules r
		LEFT JOIN sites s ON s.id = r.site_id
		LEFT JOIN sub2api_upstreams u ON u.id = r.sub2api_upstream_id
		LEFT JOIN categories c ON c.slug = r.category
		ORDER BY r.id ASC
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
			&rule.SourceName, &rule.SourceBaseURL, &rule.Category, &rule.CategoryName,
			&rule.ModelKeyword, &rule.ModelName, &rule.GroupName,
			&rule.Enabled, &rule.ScheduleEnabled, &rule.IntervalMinutes, &rule.NextRunAt, &rule.LastScheduledRunAt,
			&rule.SyncEnabled, &rule.SyncBaseGroup, &rule.SyncThresholdRatio, &rule.Sub2APIGroupName, &rule.Sub2APIGroupID,
			&rule.LastSyncAt, &rule.SyncStatus, &rule.SyncError, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
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
			r.category, COALESCE(c.name, r.category),
			r.model_keyword, r.model_name, COALESCE(r.group_name, ''), r.enabled,
			r.schedule_enabled, r.interval_minutes, r.next_run_at, r.last_scheduled_run_at,
			r.sync_enabled, r.sync_base_group, r.sync_threshold_ratio, r.sub2api_group_name, r.sub2api_group_id,
			r.last_sync_at, r.sync_status, r.sync_error, r.created_at, r.updated_at,
			COALESCE(s.id, 0), COALESCE(s.name, ''), COALESCE(s.base_url, ''), COALESCE(s.username, ''), COALESCE(s.password, ''), COALESCE(s.totp_code, ''), COALESCE(s.user_id, 0), COALESCE(s.access_token, ''), COALESCE(s.last_error, ''), s.last_run_at, COALESCE(s.created_at, now()), COALESCE(s.updated_at, now()),
			COALESCE(u.id, 0), COALESCE(u.name, ''), COALESCE(u.base_url, ''), COALESCE(u.email, ''), COALESCE(u.password, ''), COALESCE(u.auth_token, ''), COALESCE(u.totp_code, ''), COALESCE(u.last_error, ''), u.last_check_at, COALESCE(u.created_at, now()), COALESCE(u.updated_at, now())
		FROM monitor_rules r
		LEFT JOIN sites s ON s.id = r.site_id
		LEFT JOIN sub2api_upstreams u ON u.id = r.sub2api_upstream_id
		LEFT JOIN categories c ON c.slug = r.category
		WHERE r.id = $1
	`, ruleID).Scan(
		&rule.ID, &rule.SourceType, &rule.SiteID, &rule.SiteName, &rule.Sub2APIUpstreamID, &rule.Sub2APIUpstreamName,
		&rule.SourceName, &rule.SourceBaseURL, &rule.Category, &rule.CategoryName,
		&rule.ModelKeyword, &rule.ModelName, &rule.GroupName, &rule.Enabled,
		&rule.ScheduleEnabled, &rule.IntervalMinutes, &rule.NextRunAt, &rule.LastScheduledRunAt,
		&rule.SyncEnabled, &rule.SyncBaseGroup, &rule.SyncThresholdRatio, &rule.Sub2APIGroupName, &rule.Sub2APIGroupID,
		&rule.LastSyncAt, &rule.SyncStatus, &rule.SyncError, &rule.CreatedAt, &rule.UpdatedAt,
		&site.ID, &site.Name, &site.BaseURL, &site.Username, &site.Password, &site.TOTPCode, &site.UserID, &site.AccessToken, &site.LastError, &site.LastRunAt, &site.CreatedAt, &site.UpdatedAt,
		&upstream.ID, &upstream.Name, &upstream.BaseURL, &upstream.Email, &upstream.Password, &upstream.AuthToken, &upstream.TOTPCode, &upstream.LastError, &upstream.LastCheckAt, &upstream.CreatedAt, &upstream.UpdatedAt,
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

func nextScheduledRunAt(runAt time.Time, intervalMinutes int) time.Time {
	if intervalMinutes <= 0 {
		intervalMinutes = 15
	}
	return runAt.Add(time.Duration(intervalMinutes) * time.Minute)
}

func (s Store) UpdateRuleSyncStatus(ctx context.Context, ruleID int64, status string, errText string) error {
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

func (s Store) UpdateRuleSyncSuccess(ctx context.Context, ruleID int64, status string, signature string) error {
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
		status = fmt.Sprintf("paused after %d sync failures", failureCount)
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

func (s Store) InsertSnapshot(ctx context.Context, snapshot PriceSnapshot) (PriceSnapshot, error) {
	if strings.TrimSpace(snapshot.SourceType) == "" {
		snapshot.SourceType = RuleSourceNewAPI
	}
	err := s.db.QueryRow(ctx, `
		INSERT INTO price_snapshots (
			rule_id, source_type, site_id, sub2api_upstream_id, category, model_keyword, model_name, group_name, group_desc, quota_type, group_ratio,
			input_price, output_price, cache_read_price, cache_write_price, request_price, upstream_balance, balance_unit, raw
		)
		VALUES ($1, $2, NULLIF($3, 0), $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT (
			COALESCE(source_type, 'newapi'),
			COALESCE(site_id, 0),
			sub2api_upstream_id,
			category,
			model_name,
			lower(trim(group_name))
		)
		DO UPDATE SET
			rule_id = EXCLUDED.rule_id,
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
			invalid = false,
			invalid_reason = '',
			invalid_at = NULL,
			raw = EXCLUDED.raw,
			created_at = now()
		RETURNING id, created_at
	`,
		snapshot.RuleID, snapshot.SourceType, snapshot.SiteID, snapshot.Sub2APIUpstreamID, normalizeCategorySlug(snapshot.Category), snapshot.ModelKeyword,
		snapshot.ModelName, snapshot.GroupName, snapshot.GroupDesc,
		snapshot.QuotaType, snapshot.GroupRatio, snapshot.InputPrice, snapshot.OutputPrice,
		snapshot.CacheReadPrice, snapshot.CacheWritePrice, snapshot.RequestPrice, snapshot.UpstreamBalance, snapshot.BalanceUnit, string(snapshot.Raw),
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
	var groupRatio, inputPrice, outputPrice, cacheReadPrice, cacheWritePrice, requestPrice, upstreamBalance sql.NullFloat64
	var invalidAt sql.NullTime
	err := s.db.QueryRow(ctx, `
		SELECT p.id, p.rule_id, COALESCE(p.source_type, 'newapi'), COALESCE(p.site_id, 0), p.sub2api_upstream_id,
		       CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.name, '') ELSE COALESCE(s.name, '') END AS site_name,
		       CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.base_url, '') ELSE COALESCE(s.base_url, '') END AS site_base_url,
		       p.category, COALESCE(c.name, p.category) AS category_name, p.model_keyword, p.model_name,
		       p.group_name, p.group_desc, p.quota_type, p.group_ratio, p.input_price, p.output_price,
		       p.cache_read_price, p.cache_write_price, p.request_price, p.upstream_balance, p.balance_unit,
		       p.invalid, p.invalid_reason, p.invalid_at, p.raw, p.created_at
		FROM price_snapshots p
		LEFT JOIN sites s ON s.id = p.site_id
		LEFT JOIN sub2api_upstreams u ON u.id = p.sub2api_upstream_id
		LEFT JOIN categories c ON c.slug = p.category
		WHERE p.rule_id = $1 AND p.model_name = $2
		ORDER BY p.created_at DESC, p.id DESC
		LIMIT 1
	`, ruleID, strings.TrimSpace(modelName)).Scan(
		&snapshot.ID, &snapshot.RuleID, &snapshot.SourceType, &snapshot.SiteID, &snapshot.Sub2APIUpstreamID, &snapshot.SiteName, &snapshot.SiteBaseURL,
		&snapshot.Category, &snapshot.CategoryName, &snapshot.ModelKeyword, &snapshot.ModelName,
		&snapshot.GroupName, &snapshot.GroupDesc, &snapshot.QuotaType, &groupRatio, &inputPrice,
		&outputPrice, &cacheReadPrice, &cacheWritePrice, &requestPrice, &upstreamBalance, &snapshot.BalanceUnit,
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
	snapshot.InvalidAt = timePtr(invalidAt)
	return snapshot, nil
}

func (s Store) CheapestLatestSnapshot(ctx context.Context, category string, modelName string) (PriceSnapshot, error) {
	var snapshot PriceSnapshot
	var groupRatio, inputPrice, outputPrice, cacheReadPrice, cacheWritePrice, requestPrice, upstreamBalance sql.NullFloat64
	var invalidAt sql.NullTime
	err := s.db.QueryRow(ctx, `
		WITH latest AS (
			SELECT DISTINCT ON (p.rule_id)
			       p.id, p.rule_id, COALESCE(p.source_type, 'newapi') AS source_type, COALESCE(p.site_id, 0) AS site_id, p.sub2api_upstream_id,
			       CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.name, '') ELSE COALESCE(st.name, '') END AS site_name,
			       CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.base_url, '') ELSE COALESCE(st.base_url, '') END AS site_base_url,
		       r.category, COALESCE(c.name, r.category) AS category_name, p.model_keyword, p.model_name,
		       p.group_name, p.group_desc, p.quota_type, p.group_ratio, p.input_price, p.output_price,
		       p.cache_read_price, p.cache_write_price, p.request_price, p.upstream_balance, p.balance_unit,
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
			ORDER BY p.rule_id, p.created_at DESC, p.id DESC
		)
		SELECT id, rule_id, source_type, site_id, sub2api_upstream_id, site_name, site_base_url, category, category_name, model_keyword,
		       model_name, group_name, group_desc, quota_type,
		       group_ratio, input_price, output_price, cache_read_price, cache_write_price,
		       request_price, upstream_balance, balance_unit, invalid, invalid_reason, invalid_at, raw, created_at
		FROM latest
		ORDER BY COALESCE(input_price, request_price, output_price, 1e308) ASC,
		         COALESCE(output_price, 1e308) ASC,
		         group_ratio ASC NULLS LAST,
		         id DESC
		LIMIT 1
	`, normalizeCategorySlug(category), strings.TrimSpace(modelName)).Scan(
		&snapshot.ID, &snapshot.RuleID, &snapshot.SourceType, &snapshot.SiteID, &snapshot.Sub2APIUpstreamID, &snapshot.SiteName, &snapshot.SiteBaseURL,
		&snapshot.Category, &snapshot.CategoryName, &snapshot.ModelKeyword, &snapshot.ModelName,
		&snapshot.GroupName, &snapshot.GroupDesc, &snapshot.QuotaType, &groupRatio, &inputPrice,
		&outputPrice, &cacheReadPrice, &cacheWritePrice, &requestPrice, &upstreamBalance, &snapshot.BalanceUnit,
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
	snapshot.InvalidAt = timePtr(invalidAt)
	return snapshot, nil
}

func (s Store) LatestSnapshots(ctx context.Context, limit int, category string) ([]PriceSnapshot, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
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
			SELECT DISTINCT ON (COALESCE(p.source_type, 'newapi'), COALESCE(p.site_id, 0), p.sub2api_upstream_id, p.rule_category, p.model_name, lower(trim(p.group_name)))
			       p.id, p.rule_id, COALESCE(p.source_type, 'newapi') AS source_type, COALESCE(p.site_id, 0) AS site_id, p.sub2api_upstream_id,
			       CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.name, '') ELSE COALESCE(s.name, '') END AS site_name,
			       CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.base_url, '') ELSE COALESCE(s.base_url, '') END AS site_base_url,
			       p.rule_category AS category, COALESCE(c.name, p.rule_category) AS category_name, p.model_keyword, p.model_name, p.group_name,
			       p.group_desc, p.quota_type, p.group_ratio, p.input_price, p.output_price,
			       p.cache_read_price, p.cache_write_price, p.request_price, p.upstream_balance, p.balance_unit,
			       p.invalid, p.invalid_reason, p.invalid_at, p.raw, p.created_at
			FROM filtered p
			LEFT JOIN sites s ON s.id = p.site_id
			LEFT JOIN sub2api_upstreams u ON u.id = p.sub2api_upstream_id
			LEFT JOIN categories c ON c.slug = p.rule_category
			ORDER BY COALESCE(p.source_type, 'newapi'), COALESCE(p.site_id, 0), p.sub2api_upstream_id, p.rule_category, p.model_name, lower(trim(p.group_name)), p.created_at DESC, p.id DESC
		)
		SELECT id, rule_id, source_type, site_id, sub2api_upstream_id, site_name, site_base_url, category, category_name, model_keyword,
		       model_name, group_name, group_desc, quota_type,
		       group_ratio, input_price, output_price, cache_read_price, cache_write_price,
		       request_price, upstream_balance, balance_unit, invalid, invalid_reason, invalid_at, raw, created_at
		FROM latest
		ORDER BY category,
		         invalid ASC,
		         model_name,
		         COALESCE(input_price, request_price, 1e308) ASC,
		         COALESCE(output_price, 1e308) ASC,
		         group_ratio ASC NULLS LAST,
		         group_name
		LIMIT `+limitPlaceholder+`
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []PriceSnapshot
	for rows.Next() {
		var snapshot PriceSnapshot
		var groupRatio, inputPrice, outputPrice, cacheReadPrice, cacheWritePrice, requestPrice, upstreamBalance sql.NullFloat64
		var invalidAt sql.NullTime
		if err := rows.Scan(
			&snapshot.ID, &snapshot.RuleID, &snapshot.SourceType, &snapshot.SiteID, &snapshot.Sub2APIUpstreamID, &snapshot.SiteName, &snapshot.SiteBaseURL,
			&snapshot.Category, &snapshot.CategoryName, &snapshot.ModelKeyword, &snapshot.ModelName,
			&snapshot.GroupName, &snapshot.GroupDesc, &snapshot.QuotaType, &groupRatio, &inputPrice,
			&outputPrice, &cacheReadPrice, &cacheWritePrice, &requestPrice, &upstreamBalance, &snapshot.BalanceUnit,
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
		snapshot.InvalidAt = timePtr(invalidAt)
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

func (s Store) CheapestSyncCandidate(ctx context.Context, category string, modelName string) (PriceSnapshot, []PriceSnapshot, error) {
	candidates, skipped, err := s.SyncCandidates(ctx, category, modelName)
	if err != nil {
		return PriceSnapshot{}, nil, err
	}
	if len(candidates) == 0 {
		return PriceSnapshot{}, skipped, pgx.ErrNoRows
	}
	return candidates[0], skipped, nil
}

func (s Store) SyncCandidates(ctx context.Context, category string, modelName string) ([]PriceSnapshot, []PriceSnapshot, error) {
	snapshots, err := s.latestSnapshotsForModel(ctx, category, modelName)
	if err != nil {
		return nil, nil, err
	}
	candidates := make([]PriceSnapshot, 0, len(snapshots))
	skipped := make([]PriceSnapshot, 0)
	for _, snapshot := range snapshots {
		if snapshotBalanceInsufficient(snapshot) {
			skipped = append(skipped, snapshot)
			continue
		}
		candidates = append(candidates, snapshot)
	}
	return candidates, skipped, nil
}

func (s Store) latestSnapshotsForModel(ctx context.Context, category string, modelName string) ([]PriceSnapshot, error) {
	rows, err := s.db.Query(ctx, `
		WITH latest AS (
			SELECT DISTINCT ON (p.rule_id)
			       p.id, p.rule_id, COALESCE(p.source_type, 'newapi') AS source_type, COALESCE(p.site_id, 0) AS site_id, p.sub2api_upstream_id,
			       CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.name, '') ELSE COALESCE(st.name, '') END AS site_name,
			       CASE WHEN COALESCE(p.source_type, 'newapi') = 'sub2api' THEN COALESCE(u.base_url, '') ELSE COALESCE(st.base_url, '') END AS site_base_url,
			       r.category, COALESCE(c.name, r.category) AS category_name, p.model_keyword, p.model_name,
			       p.group_name, p.group_desc, p.quota_type, p.group_ratio, p.input_price, p.output_price,
			       p.cache_read_price, p.cache_write_price, p.request_price, p.upstream_balance, p.balance_unit,
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
			ORDER BY p.rule_id, p.created_at DESC, p.id DESC
		)
		SELECT id, rule_id, source_type, site_id, sub2api_upstream_id, site_name, site_base_url, category, category_name, model_keyword,
		       model_name, group_name, group_desc, quota_type,
		       group_ratio, input_price, output_price, cache_read_price, cache_write_price,
		       request_price, upstream_balance, balance_unit, invalid, invalid_reason, invalid_at, raw, created_at
		FROM latest
		ORDER BY COALESCE(input_price, request_price, output_price, 1e308) ASC,
		         COALESCE(output_price, 1e308) ASC,
		         group_ratio ASC NULLS LAST,
		         id DESC
	`, normalizeCategorySlug(category), strings.TrimSpace(modelName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []PriceSnapshot
	for rows.Next() {
		var snapshot PriceSnapshot
		var groupRatio, inputPrice, outputPrice, cacheReadPrice, cacheWritePrice, requestPrice, upstreamBalance sql.NullFloat64
		var invalidAt sql.NullTime
		if err := rows.Scan(
			&snapshot.ID, &snapshot.RuleID, &snapshot.SourceType, &snapshot.SiteID, &snapshot.Sub2APIUpstreamID, &snapshot.SiteName, &snapshot.SiteBaseURL,
			&snapshot.Category, &snapshot.CategoryName, &snapshot.ModelKeyword, &snapshot.ModelName,
			&snapshot.GroupName, &snapshot.GroupDesc, &snapshot.QuotaType, &groupRatio, &inputPrice,
			&outputPrice, &cacheReadPrice, &cacheWritePrice, &requestPrice, &upstreamBalance, &snapshot.BalanceUnit,
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
		snapshot.InvalidAt = timePtr(invalidAt)
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

func snapshotBalanceInsufficient(snapshot PriceSnapshot) bool {
	return snapshot.UpstreamBalance != nil && *snapshot.UpstreamBalance <= 0
}

func (s Store) GetIntegrationSettings(ctx context.Context) (IntegrationSettings, error) {
	var settings IntegrationSettings
	var syncThresholdRatio sql.NullFloat64
	err := s.db.QueryRow(ctx, `
		SELECT sub2api_enabled, sub2api_main_base_url, sub2api_admin_key,
		       sub2api_base_url, sub2api_access_token, sub2api_email, sub2api_password,
		       sync_threshold_ratio,
		       email_notify_enabled, email_notify_price_change, email_notify_sync_update,
		       smtp_host, smtp_port, smtp_encryption, smtp_username, smtp_password, smtp_from, smtp_to,
		       updated_at
		FROM integration_settings
		WHERE id = true
	`).Scan(
		&settings.Sub2APIEnabled, &settings.Sub2APIMainBaseURL, &settings.Sub2APIAdminKey,
		&settings.Sub2APIBaseURL, &settings.Sub2APIAccessToken,
		&settings.Sub2APIEmail, &settings.Sub2APIPassword,
		&syncThresholdRatio,
		&settings.EmailNotifyEnabled, &settings.EmailNotifyPriceChange, &settings.EmailNotifySyncUpdate,
		&settings.SMTPHost, &settings.SMTPPort, &settings.SMTPEncryption, &settings.SMTPUsername, &settings.SMTPPassword,
		&settings.SMTPFrom, &settings.SMTPTo, &settings.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return IntegrationSettings{}, nil
	}
	settings.SyncThresholdRatio = floatPtr(syncThresholdRatio)
	if settings.Sub2APIMainBaseURL == "" {
		settings.Sub2APIMainBaseURL = settings.Sub2APIBaseURL
	}
	if settings.Sub2APIAdminKey == "" {
		settings.Sub2APIAdminKey = settings.Sub2APIAccessToken
	}
	settings.SMTPEncryption = normalizeSMTPEncryption(settings.SMTPEncryption)
	settings.Sub2APIBaseURL = settings.Sub2APIMainBaseURL
	settings.Sub2APIAccessToken = settings.Sub2APIAdminKey
	return settings, err
}

func (s Store) SaveIntegrationSettings(ctx context.Context, input SettingsInput) (IntegrationSettings, error) {
	input.Sub2APIMainBaseURL = normalizeBaseURL(firstNonEmpty(input.Sub2APIMainBaseURL, input.Sub2APIBaseURL))
	input.Sub2APIAdminKey = strings.TrimSpace(firstNonEmpty(input.Sub2APIAdminKey, input.Sub2APIAccessToken))
	input.Sub2APIEmail = strings.TrimSpace(input.Sub2APIEmail)
	input.SMTPHost = strings.TrimSpace(input.SMTPHost)
	input.SMTPEncryption = normalizeSMTPEncryption(input.SMTPEncryption)
	input.SMTPUsername = strings.TrimSpace(input.SMTPUsername)
	input.SMTPFrom = strings.TrimSpace(input.SMTPFrom)
	input.SMTPTo = strings.TrimSpace(input.SMTPTo)
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
	err := s.db.QueryRow(ctx, `
		INSERT INTO integration_settings (
			id, sub2api_enabled, sub2api_main_base_url, sub2api_admin_key,
			sub2api_base_url, sub2api_access_token, sub2api_email, sub2api_password,
			sync_threshold_ratio,
			email_notify_enabled, email_notify_price_change, email_notify_sync_update,
			smtp_host, smtp_port, smtp_encryption, smtp_username, smtp_password, smtp_from, smtp_to,
			updated_at
		)
		VALUES (true, $1, $2, $3, $2, $3, $4, $5, CASE WHEN $6::double precision > 0 THEN $6::double precision ELSE NULL END, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, now())
		ON CONFLICT (id) DO UPDATE
		SET sub2api_enabled = EXCLUDED.sub2api_enabled,
		    sub2api_main_base_url = EXCLUDED.sub2api_main_base_url,
		    sub2api_admin_key = EXCLUDED.sub2api_admin_key,
		    sub2api_base_url = EXCLUDED.sub2api_base_url,
		    sub2api_access_token = EXCLUDED.sub2api_access_token,
		    sub2api_email = EXCLUDED.sub2api_email,
		    sub2api_password = EXCLUDED.sub2api_password,
		    sync_threshold_ratio = EXCLUDED.sync_threshold_ratio,
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
		    updated_at = now()
		RETURNING sub2api_enabled, sub2api_main_base_url, sub2api_admin_key,
		          sub2api_base_url, sub2api_access_token, sub2api_email, sub2api_password,
		          sync_threshold_ratio,
		          email_notify_enabled, email_notify_price_change, email_notify_sync_update,
		          smtp_host, smtp_port, smtp_encryption, smtp_username, smtp_password, smtp_from, smtp_to,
		          updated_at
	`,
		input.Sub2APIEnabled, input.Sub2APIMainBaseURL, input.Sub2APIAdminKey, input.Sub2APIEmail, input.Sub2APIPassword,
		input.SyncThresholdRatio, input.EmailNotifyEnabled, input.EmailNotifyPriceChange, input.EmailNotifySyncUpdate,
		input.SMTPHost, input.SMTPPort, input.SMTPEncryption, input.SMTPUsername, input.SMTPPassword, input.SMTPFrom, input.SMTPTo,
	).Scan(
		&settings.Sub2APIEnabled, &settings.Sub2APIMainBaseURL, &settings.Sub2APIAdminKey,
		&settings.Sub2APIBaseURL, &settings.Sub2APIAccessToken,
		&settings.Sub2APIEmail, &settings.Sub2APIPassword,
		&syncThresholdRatio,
		&settings.EmailNotifyEnabled, &settings.EmailNotifyPriceChange, &settings.EmailNotifySyncUpdate,
		&settings.SMTPHost, &settings.SMTPPort, &settings.SMTPEncryption, &settings.SMTPUsername, &settings.SMTPPassword,
		&settings.SMTPFrom, &settings.SMTPTo, &settings.UpdatedAt,
	)
	settings.SyncThresholdRatio = floatPtr(syncThresholdRatio)
	return settings, err
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
	input.TOTPCode = strings.TrimSpace(input.TOTPCode)
	return input
}
