CREATE TABLE IF NOT EXISTS sites (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  base_url TEXT NOT NULL,
  username TEXT NOT NULL,
  password TEXT NOT NULL,
  totp_code TEXT NOT NULL DEFAULT '',
  user_id BIGINT NOT NULL DEFAULT 0,
  access_token TEXT NOT NULL DEFAULT '',
  cookie_jar TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  last_run_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS categories (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT NOT NULL UNIQUE,
  sub2api_main_group_id BIGINT NOT NULL DEFAULT 0,
  sub2api_main_group_name TEXT NOT NULL DEFAULT '',
  sub2api_main_groups JSONB NOT NULL DEFAULT '[]'::jsonb,
  sub2api_main_group_keywords JSONB NOT NULL DEFAULT '[]'::jsonb,
  blocked_group_keywords JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS monitor_rules (
  id BIGSERIAL PRIMARY KEY,
  source_type TEXT NOT NULL DEFAULT 'newapi',
  site_id BIGINT REFERENCES sites(id) ON DELETE CASCADE,
  category TEXT NOT NULL DEFAULT 'other',
  model_keyword TEXT NOT NULL DEFAULT '',
  model_name TEXT NOT NULL,
  group_name TEXT NOT NULL DEFAULT '',
  enabled BOOLEAN NOT NULL DEFAULT true,
  schedule_enabled BOOLEAN NOT NULL DEFAULT true,
  interval_minutes INTEGER NOT NULL DEFAULT 15,
  next_run_at TIMESTAMPTZ,
  last_scheduled_run_at TIMESTAMPTZ,
  sync_enabled BOOLEAN NOT NULL DEFAULT false,
  sync_base_group TEXT NOT NULL DEFAULT '',
  sync_threshold_ratio DOUBLE PRECISION,
  sub2api_group_name TEXT NOT NULL DEFAULT '',
  sub2api_group_id BIGINT NOT NULL DEFAULT 0,
  last_sync_at TIMESTAMPTZ,
  sync_status TEXT NOT NULL DEFAULT '',
  sync_error TEXT NOT NULL DEFAULT '',
  sync_signature TEXT NOT NULL DEFAULT '',
  sync_failure_count INTEGER NOT NULL DEFAULT 0,
  sync_failure_signature TEXT NOT NULL DEFAULT '',
  checkin_enabled BOOLEAN NOT NULL DEFAULT false,
  checkin_status TEXT NOT NULL DEFAULT '',
  checkin_reward DOUBLE PRECISION,
  checkin_reward_unit TEXT NOT NULL DEFAULT '',
  checkin_message TEXT NOT NULL DEFAULT '',
  checkin_checked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS integration_settings (
  id BOOLEAN PRIMARY KEY DEFAULT true,
  sub2api_enabled BOOLEAN NOT NULL DEFAULT false,
  sub2api_main_base_url TEXT NOT NULL DEFAULT '',
  sub2api_admin_key TEXT NOT NULL DEFAULT '',
  sub2api_base_url TEXT NOT NULL DEFAULT '',
  sub2api_access_token TEXT NOT NULL DEFAULT '',
  sub2api_email TEXT NOT NULL DEFAULT '',
  sub2api_password TEXT NOT NULL DEFAULT '',
  sub2api_sync_account_mode TEXT NOT NULL DEFAULT 'schedulable_only',
  monitor_interval_minutes INTEGER NOT NULL DEFAULT 15,
  monitor_rule_delay_seconds INTEGER NOT NULL DEFAULT 60,
  latency_test_enabled BOOLEAN NOT NULL DEFAULT false,
  latency_weight_per_second DOUBLE PRECISION NOT NULL DEFAULT 0.1,
  expected_cache_hit_ratio DOUBLE PRECISION NOT NULL DEFAULT 0,
  upstream_balance_threshold DOUBLE PRECISION NOT NULL DEFAULT 0,
  sync_threshold_ratio DOUBLE PRECISION,
  sync_threshold_ratios JSONB NOT NULL DEFAULT '{}'::jsonb,
  email_notify_enabled BOOLEAN NOT NULL DEFAULT false,
  email_notify_price_change BOOLEAN NOT NULL DEFAULT true,
  email_notify_sync_update BOOLEAN NOT NULL DEFAULT true,
  smtp_host TEXT NOT NULL DEFAULT '',
  smtp_port INTEGER NOT NULL DEFAULT 587,
  smtp_encryption TEXT NOT NULL DEFAULT 'auto',
  smtp_username TEXT NOT NULL DEFAULT '',
  smtp_password TEXT NOT NULL DEFAULT '',
  smtp_from TEXT NOT NULL DEFAULT '',
  smtp_to TEXT NOT NULL DEFAULT '',
  email_template_enabled BOOLEAN NOT NULL DEFAULT false,
  email_template_subject TEXT NOT NULL DEFAULT '',
  email_template_body TEXT NOT NULL DEFAULT '',
  email_template_configs JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT integration_settings_singleton CHECK (id)
);

CREATE TABLE IF NOT EXISTS admin_credentials (
  id BOOLEAN PRIMARY KEY DEFAULT true,
  username TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT admin_credentials_singleton CHECK (id)
);

CREATE TABLE IF NOT EXISTS sub2api_upstreams (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  base_url TEXT NOT NULL,
  email TEXT NOT NULL DEFAULT '',
  password TEXT NOT NULL DEFAULT '',
  auth_token TEXT NOT NULL DEFAULT '',
  totp_code TEXT NOT NULL DEFAULT '',
  cookie_jar TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  last_check_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS low_balance_notifications (
  signature TEXT PRIMARY KEY,
  notified_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS sub2api_main_base_url TEXT NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS sub2api_admin_key TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS sub2api_sync_account_mode TEXT NOT NULL DEFAULT 'schedulable_only';
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS sync_threshold_ratio DOUBLE PRECISION;
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS sync_threshold_ratios JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS email_notify_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS email_notify_price_change BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS email_notify_sync_update BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS monitor_interval_minutes INTEGER NOT NULL DEFAULT 15;
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS monitor_rule_delay_seconds INTEGER NOT NULL DEFAULT 60;
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS latency_test_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS latency_weight_per_second DOUBLE PRECISION NOT NULL DEFAULT 0.1;
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS expected_cache_hit_ratio DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS sites
  ADD COLUMN IF NOT EXISTS cookie_jar TEXT NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS sub2api_upstreams
  ADD COLUMN IF NOT EXISTS cookie_jar TEXT NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS smtp_host TEXT NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS smtp_port INTEGER NOT NULL DEFAULT 587;
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS smtp_encryption TEXT NOT NULL DEFAULT 'auto';
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS smtp_username TEXT NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS smtp_password TEXT NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS smtp_from TEXT NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS smtp_to TEXT NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS email_template_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS email_template_subject TEXT NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS email_template_body TEXT NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS email_template_configs JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE IF EXISTS integration_settings
  ADD COLUMN IF NOT EXISTS upstream_balance_threshold DOUBLE PRECISION NOT NULL DEFAULT 0;
UPDATE integration_settings
SET sub2api_main_base_url = COALESCE(NULLIF(sub2api_main_base_url, ''), sub2api_base_url),
    sub2api_admin_key = COALESCE(NULLIF(sub2api_admin_key, ''), sub2api_access_token)
WHERE id = true;

UPDATE integration_settings
SET sync_threshold_ratios = jsonb_build_object('codex', sync_threshold_ratio, 'claud', sync_threshold_ratio)
WHERE id = true
  AND sync_threshold_ratio IS NOT NULL
  AND sync_threshold_ratio > 0
  AND sync_threshold_ratios = '{}'::jsonb;

ALTER TABLE IF EXISTS categories
  ADD COLUMN IF NOT EXISTS sub2api_main_group_id BIGINT NOT NULL DEFAULT 0;

ALTER TABLE IF EXISTS categories
  ADD COLUMN IF NOT EXISTS sub2api_main_group_name TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS categories
  ADD COLUMN IF NOT EXISTS sub2api_main_groups JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE IF EXISTS categories
  ADD COLUMN IF NOT EXISTS sub2api_main_group_keywords JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE IF EXISTS categories
  ADD COLUMN IF NOT EXISTS blocked_group_keywords JSONB NOT NULL DEFAULT '[]'::jsonb;

UPDATE categories
SET sub2api_main_groups = jsonb_build_array(jsonb_build_object('id', sub2api_main_group_id, 'name', sub2api_main_group_name))
WHERE sub2api_main_groups = '[]'::jsonb
  AND (sub2api_main_group_id > 0 OR sub2api_main_group_name <> '');

CREATE TABLE IF NOT EXISTS price_snapshots (
  id BIGSERIAL PRIMARY KEY,
  rule_id BIGINT NOT NULL REFERENCES monitor_rules(id) ON DELETE CASCADE,
  source_type TEXT NOT NULL DEFAULT 'newapi',
  site_id BIGINT REFERENCES sites(id) ON DELETE SET NULL,
  sub2api_upstream_id BIGINT NOT NULL DEFAULT 0,
  source_base_url TEXT NOT NULL DEFAULT '',
  source_account TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT 'other',
  model_keyword TEXT NOT NULL DEFAULT '',
  model_name TEXT NOT NULL,
  group_name TEXT NOT NULL,
  group_desc TEXT NOT NULL DEFAULT '',
  quota_type INTEGER NOT NULL DEFAULT 0,
  group_ratio DOUBLE PRECISION,
  input_price DOUBLE PRECISION,
  output_price DOUBLE PRECISION,
  cache_read_price DOUBLE PRECISION,
  cache_write_price DOUBLE PRECISION,
  request_price DOUBLE PRECISION,
  request_latency_ms DOUBLE PRECISION,
  upstream_balance DOUBLE PRECISION,
  balance_unit TEXT NOT NULL DEFAULT '',
  online_topup_enabled BOOLEAN NOT NULL DEFAULT false,
  recharge_multiplier DOUBLE PRECISION,
  invalid BOOLEAN NOT NULL DEFAULT false,
  invalid_reason TEXT NOT NULL DEFAULT '',
  invalid_at TIMESTAMPTZ,
  raw JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE IF EXISTS price_snapshots
  ADD COLUMN IF NOT EXISTS upstream_balance DOUBLE PRECISION;

ALTER TABLE IF EXISTS price_snapshots
  ADD COLUMN IF NOT EXISTS balance_unit TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS price_snapshots
  ADD COLUMN IF NOT EXISTS online_topup_enabled BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE IF EXISTS price_snapshots
  ADD COLUMN IF NOT EXISTS recharge_multiplier DOUBLE PRECISION;

ALTER TABLE IF EXISTS price_snapshots
  ADD COLUMN IF NOT EXISTS invalid BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE IF EXISTS price_snapshots
  ADD COLUMN IF NOT EXISTS invalid_reason TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS price_snapshots
  ADD COLUMN IF NOT EXISTS invalid_at TIMESTAMPTZ;

ALTER TABLE IF EXISTS price_snapshots
  ADD COLUMN IF NOT EXISTS request_latency_ms DOUBLE PRECISION;

UPDATE price_snapshots
SET upstream_balance = upstream_balance / 500000.0,
    balance_unit = 'usd'
WHERE lower(trim(balance_unit)) = 'quota'
  AND upstream_balance IS NOT NULL;

UPDATE price_snapshots
SET balance_unit = 'usd'
WHERE lower(trim(balance_unit)) = 'balance'
  AND upstream_balance IS NOT NULL;

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT 'newapi';

ALTER TABLE IF EXISTS monitor_rules
  ALTER COLUMN site_id DROP NOT NULL;

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'other';

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS model_keyword TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS monitor_rules
  ALTER COLUMN group_name SET DEFAULT '';

ALTER TABLE IF EXISTS monitor_rules
  ALTER COLUMN group_name DROP NOT NULL;

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS schedule_enabled BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS interval_minutes INTEGER NOT NULL DEFAULT 15;

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS next_run_at TIMESTAMPTZ;

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS last_scheduled_run_at TIMESTAMPTZ;

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS sync_enabled BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS sync_base_group TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS sync_threshold_ratio DOUBLE PRECISION;

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS sub2api_group_name TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS sub2api_group_id BIGINT NOT NULL DEFAULT 0;

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS sub2api_upstream_id BIGINT NOT NULL DEFAULT 0;

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS last_sync_at TIMESTAMPTZ;

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS sync_status TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS sync_error TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS sync_signature TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS sync_failure_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS sync_failure_signature TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS checkin_enabled BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS checkin_status TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS checkin_reward DOUBLE PRECISION;

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS checkin_reward_unit TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS checkin_message TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS monitor_rules
  ADD COLUMN IF NOT EXISTS checkin_checked_at TIMESTAMPTZ;

UPDATE monitor_rules
SET sync_base_group = COALESCE(group_name, '')
WHERE sync_base_group = ''
  AND COALESCE(group_name, '') <> '';

ALTER TABLE IF EXISTS price_snapshots
  ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT 'newapi';

ALTER TABLE IF EXISTS price_snapshots
  ADD COLUMN IF NOT EXISTS sub2api_upstream_id BIGINT NOT NULL DEFAULT 0;

ALTER TABLE IF EXISTS price_snapshots
  ADD COLUMN IF NOT EXISTS source_base_url TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS price_snapshots
  ADD COLUMN IF NOT EXISTS source_account TEXT NOT NULL DEFAULT '';

ALTER TABLE IF EXISTS price_snapshots
  ALTER COLUMN site_id DROP NOT NULL;

ALTER TABLE IF EXISTS price_snapshots
  ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'other';

UPDATE monitor_rules
SET source_type = CASE WHEN COALESCE(sub2api_upstream_id, 0) > 0 AND COALESCE(site_id, 0) = 0 THEN 'sub2api' ELSE COALESCE(NULLIF(source_type, ''), 'newapi') END;

UPDATE price_snapshots
SET source_type = COALESCE(NULLIF(source_type, ''), 'newapi');

UPDATE price_snapshots p
SET source_base_url = COALESCE(s.base_url, ''),
    source_account = COALESCE(s.username, '')
FROM sites s
WHERE p.site_id = s.id
  AND (p.source_base_url = '' OR p.source_account = '');

UPDATE price_snapshots p
SET source_base_url = COALESCE(u.base_url, ''),
    source_account = CASE
      WHEN trim(COALESCE(u.email, '')) <> '' THEN trim(u.email)
      WHEN trim(COALESCE(u.auth_token, '')) <> '' THEN 'token:' || left(md5(u.auth_token), 12)
      ELSE ''
    END
FROM sub2api_upstreams u
WHERE p.sub2api_upstream_id = u.id
  AND COALESCE(p.source_type, 'newapi') = 'sub2api'
  AND (p.source_base_url = '' OR p.source_account = '');

ALTER TABLE IF EXISTS price_snapshots
  ADD COLUMN IF NOT EXISTS model_keyword TEXT NOT NULL DEFAULT '';

INSERT INTO categories (name, slug)
VALUES ('Codex', 'codex'), ('Claude', 'claud'), ('其他', 'other')
ON CONFLICT (slug) DO UPDATE
SET name = CASE
    WHEN categories.slug = 'claud' AND lower(trim(categories.name)) IN ('', 'claud') THEN 'Claude'
    WHEN trim(categories.name) = '' THEN EXCLUDED.name
    ELSE categories.name
  END,
  updated_at = CASE
    WHEN (categories.slug = 'claud' AND lower(trim(categories.name)) IN ('', 'claud'))
      OR trim(categories.name) = ''
    THEN now()
    ELSE categories.updated_at
  END;

UPDATE monitor_rules
SET category = 'claud'
WHERE lower(trim(category)) = 'claude';

UPDATE price_snapshots
SET category = 'claud'
WHERE lower(trim(category)) = 'claude';

INSERT INTO categories (name, slug)
SELECT DISTINCT category,
       CASE lower(trim(category))
         WHEN 'claude' THEN 'claud'
         ELSE lower(trim(category))
       END
FROM monitor_rules
WHERE trim(category) <> ''
  AND NOT EXISTS (
    SELECT 1
    FROM categories c
    WHERE c.slug = CASE lower(trim(monitor_rules.category))
      WHEN 'claude' THEN 'claud'
      ELSE lower(trim(monitor_rules.category))
    END
  )
ON CONFLICT (slug) DO NOTHING;

UPDATE monitor_rules
SET model_keyword = model_name
WHERE model_keyword = '';

UPDATE price_snapshots p
SET model_keyword = r.model_keyword
FROM monitor_rules r
WHERE p.rule_id = r.id
  AND p.model_keyword = '';

DELETE FROM price_snapshots p
USING monitor_rules r
WHERE p.rule_id = r.id
  AND lower(trim(p.model_name)) <> lower(trim(r.model_keyword));

DELETE FROM price_snapshots
WHERE upstream_balance IS NULL;

DELETE FROM price_snapshots
WHERE invalid = true
  AND invalid_at IS NOT NULL
  AND invalid_at < now() - interval '7 days';

DELETE FROM price_snapshots p
USING (
  SELECT p.id,
         row_number() OVER (
           PARTITION BY COALESCE(p.source_type, 'newapi'), lower(regexp_replace(trim(p.source_base_url), '/+$', '')), lower(trim(p.source_account)),
                        r.category, p.model_name, lower(trim(p.group_name))
           ORDER BY p.created_at DESC, p.id DESC
         ) AS duplicate_rank
  FROM price_snapshots p
  JOIN monitor_rules r ON r.id = p.rule_id
) ranked
WHERE p.id = ranked.id
  AND ranked.duplicate_rank > 1;

UPDATE price_snapshots p
SET category = r.category
FROM monitor_rules r
WHERE p.rule_id = r.id
  AND p.category <> r.category;

ALTER TABLE IF EXISTS monitor_rules
  DROP CONSTRAINT IF EXISTS monitor_rules_site_id_model_name_group_name_key;

ALTER TABLE IF EXISTS monitor_rules
  DROP CONSTRAINT IF EXISTS monitor_rules_site_id_category_model_name_group_name_key;

DROP INDEX IF EXISTS idx_monitor_rules_site_category_model_group;

CREATE INDEX IF NOT EXISTS idx_monitor_rules_site_category_model_group
  ON monitor_rules (site_id, category, model_name, group_name);

CREATE INDEX IF NOT EXISTS idx_monitor_rules_site_category_keyword
  ON monitor_rules (site_id, category, model_keyword);

CREATE INDEX IF NOT EXISTS idx_monitor_rules_site_id ON monitor_rules (site_id);
CREATE INDEX IF NOT EXISTS idx_monitor_rules_sub2api_upstream_id ON monitor_rules (sub2api_upstream_id);
CREATE INDEX IF NOT EXISTS idx_monitor_rules_schedule_due
  ON monitor_rules (enabled, schedule_enabled, next_run_at);
CREATE INDEX IF NOT EXISTS idx_sub2api_upstreams_base_url ON sub2api_upstreams (base_url);
CREATE INDEX IF NOT EXISTS idx_price_snapshots_rule_created ON price_snapshots (rule_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_price_snapshots_site_created ON price_snapshots (site_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_price_snapshots_category_model_price
  ON price_snapshots (category, model_name, input_price);
CREATE INDEX IF NOT EXISTS idx_price_snapshots_rule_model_created
  ON price_snapshots (rule_id, model_name, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_price_snapshots_invalid_at
  ON price_snapshots (invalid, invalid_at);

DROP INDEX IF EXISTS idx_price_snapshots_unique_source_group_model;
DROP INDEX IF EXISTS idx_price_snapshots_unique_source_account_group_model;

DELETE FROM price_snapshots p
USING (
  SELECT id,
         row_number() OVER (
           PARTITION BY COALESCE(source_type, 'newapi'), lower(regexp_replace(trim(source_base_url), '/+$', '')), lower(trim(source_account)),
                        category, model_name, lower(trim(group_name))
           ORDER BY created_at DESC, id DESC
         ) AS duplicate_rank
  FROM price_snapshots
) ranked
WHERE p.id = ranked.id
  AND ranked.duplicate_rank > 1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_price_snapshots_unique_source_account_group_model
  ON price_snapshots (
    COALESCE(source_type, 'newapi'),
    lower(regexp_replace(trim(source_base_url), '/+$', '')),
    lower(trim(source_account)),
    category,
    model_name,
    lower(trim(group_name))
  );
