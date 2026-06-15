package app

import "time"

type Config struct {
	Addr            string
	DatabaseURL     string
	BasicAuthUser   string
	BasicAuthPass   string
	SessionSecret   string
	MonitorInterval time.Duration
}

type AdminCredentials struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Site struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	BaseURL     string     `json:"base_url"`
	Username    string     `json:"username"`
	Password    string     `json:"-"`
	TOTPCode    string     `json:"-"`
	UserID      int64      `json:"-"`
	AccessToken string     `json:"-"`
	CookieJar   string     `json:"-"`
	LastError   string     `json:"last_error"`
	LastRunAt   *time.Time `json:"last_run_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Category struct {
	ID                   int64             `json:"id"`
	Name                 string            `json:"name"`
	Slug                 string            `json:"slug"`
	Sub2APIMainGroupID   int64             `json:"sub2api_main_group_id"`
	Sub2APIMainGroupName string            `json:"sub2api_main_group_name"`
	Sub2APIMainGroups    []Sub2APIGroupRef `json:"sub2api_main_groups"`
	BlockedGroupKeywords []string          `json:"blocked_group_keywords"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

type Sub2APIGroupRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Rule struct {
	ID                  int64      `json:"id"`
	SourceType          string     `json:"source_type"`
	SiteID              int64      `json:"site_id"`
	SiteName            string     `json:"site_name"`
	SourceName          string     `json:"source_name"`
	SourceBaseURL       string     `json:"source_base_url"`
	SourceAccount       string     `json:"source_account"`
	Sub2APIUpstreamID   int64      `json:"sub2api_upstream_id"`
	Sub2APIUpstreamName string     `json:"sub2api_upstream_name"`
	Category            string     `json:"category"`
	CategoryName        string     `json:"category_name"`
	ModelKeyword        string     `json:"model_keyword"`
	ModelName           string     `json:"model_name"`
	GroupName           string     `json:"group_name"`
	Enabled             bool       `json:"enabled"`
	ScheduleEnabled     bool       `json:"schedule_enabled"`
	IntervalMinutes     int        `json:"interval_minutes"`
	NextRunAt           *time.Time `json:"next_run_at"`
	LastScheduledRunAt  *time.Time `json:"last_scheduled_run_at"`
	SyncEnabled         bool       `json:"sync_enabled"`
	SyncBaseGroup       string     `json:"sync_base_group"`
	SyncThresholdRatio  *float64   `json:"sync_threshold_ratio"`
	Sub2APIGroupName    string     `json:"sub2api_group_name"`
	Sub2APIGroupID      int64      `json:"sub2api_group_id"`
	LastSyncAt          *time.Time `json:"last_sync_at"`
	SyncStatus          string     `json:"sync_status"`
	SyncError           string     `json:"sync_error"`
	UpstreamBalance     *float64   `json:"upstream_balance"`
	BalanceUnit         string     `json:"balance_unit"`
	CheckinEnabled      bool       `json:"checkin_enabled"`
	CheckinStatus       string     `json:"checkin_status"`
	CheckinReward       *float64   `json:"checkin_reward"`
	CheckinRewardUnit   string     `json:"checkin_reward_unit"`
	CheckinMessage      string     `json:"checkin_message"`
	CheckinCheckedAt    *time.Time `json:"checkin_checked_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type Sub2APIUpstream struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	BaseURL     string     `json:"base_url"`
	Email       string     `json:"email"`
	AuthToken   string     `json:"auth_token,omitempty"`
	Password    string     `json:"password,omitempty"`
	TOTPCode    string     `json:"totp_code,omitempty"`
	CookieJar   string     `json:"-"`
	LastError   string     `json:"last_error"`
	LastCheckAt *time.Time `json:"last_check_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type PriceSnapshot struct {
	ID                 int64      `json:"id"`
	RuleID             int64      `json:"rule_id"`
	SourceType         string     `json:"source_type"`
	SiteID             int64      `json:"site_id"`
	Sub2APIUpstreamID  int64      `json:"sub2api_upstream_id"`
	SiteName           string     `json:"site_name"`
	SiteBaseURL        string     `json:"site_base_url"`
	SourceAccount      string     `json:"source_account"`
	Category           string     `json:"category"`
	CategoryName       string     `json:"category_name"`
	ModelKeyword       string     `json:"model_keyword"`
	ModelName          string     `json:"model_name"`
	GroupName          string     `json:"group_name"`
	GroupDesc          string     `json:"group_desc"`
	QuotaType          int        `json:"quota_type"`
	GroupRatio         *float64   `json:"group_ratio"`
	InputPrice         *float64   `json:"input_price"`
	OutputPrice        *float64   `json:"output_price"`
	CacheReadPrice     *float64   `json:"cache_read_price"`
	CacheWritePrice    *float64   `json:"cache_write_price"`
	RequestPrice       *float64   `json:"request_price"`
	RequestLatencyMS   *float64   `json:"request_latency_ms"`
	UpstreamBalance    *float64   `json:"upstream_balance"`
	BalanceUnit        string     `json:"balance_unit"`
	OnlineTopupEnabled bool       `json:"online_topup_enabled"`
	RechargeMultiplier *float64   `json:"recharge_multiplier"`
	Invalid            bool       `json:"invalid"`
	InvalidReason      string     `json:"invalid_reason"`
	InvalidAt          *time.Time `json:"invalid_at"`
	Raw                []byte     `json:"-"`
	CreatedAt          time.Time  `json:"created_at"`
}

type PricingRow struct {
	ModelName        string   `json:"model"`
	GroupName        string   `json:"group"`
	GroupDesc        string   `json:"group_desc"`
	QuotaType        int      `json:"quota_type"`
	GroupRatio       float64  `json:"group_ratio"`
	InputPrice       *float64 `json:"input_price"`
	OutputPrice      *float64 `json:"output_price"`
	CacheReadPrice   *float64 `json:"cache_read_price"`
	CacheWritePrice  *float64 `json:"cache_write_price"`
	RequestPrice     *float64 `json:"request_price"`
	RequestLatencyMS *float64 `json:"request_latency_ms"`
}

type NewAPIUserGroupPricing struct {
	Desc  string
	Ratio *float64
}

type UpstreamBalance struct {
	Value *float64 `json:"value"`
	Unit  string   `json:"unit"`
}

type RechargeStatus struct {
	Enabled    bool     `json:"enabled"`
	Multiplier *float64 `json:"multiplier"`
}

type CheckinResult struct {
	Enabled   bool
	Status    string
	Reward    *float64
	Unit      string
	Message   string
	CheckedAt time.Time
}

const newAPIQuotaPerUSD = 500 * 1000.0

func newAPIQuotaToUSD(quota float64) float64 {
	return quota / newAPIQuotaPerUSD
}

type Sub2APIUserPriceRow struct {
	ModelName                      string   `json:"model"`
	Provider                       string   `json:"provider"`
	Mode                           string   `json:"mode"`
	GroupID                        int64    `json:"group_id"`
	GroupName                      string   `json:"group_name"`
	GroupPlatform                  string   `json:"group_platform"`
	GroupDefaultRate               *float64 `json:"group_default_rate"`
	UserGroupRate                  *float64 `json:"user_group_rate"`
	EffectiveRate                  float64  `json:"effective_rate"`
	OfficialInputPerMillion        *float64 `json:"official_input_per_1m_tokens"`
	OfficialOutputPerMillion       *float64 `json:"official_output_per_1m_tokens"`
	OfficialCacheWritePerMillion   *float64 `json:"official_cache_write_per_1m_tokens"`
	OfficialCacheWrite1hPerMillion *float64 `json:"official_cache_write_1h_per_1m_tokens"`
	OfficialCacheReadPerMillion    *float64 `json:"official_cache_read_per_1m_tokens"`
	FinalInputPerMillion           *float64 `json:"final_input_per_1m_tokens"`
	FinalOutputPerMillion          *float64 `json:"final_output_per_1m_tokens"`
	FinalCacheWritePerMillion      *float64 `json:"final_cache_write_per_1m_tokens"`
	FinalCacheWrite1hPerMillion    *float64 `json:"final_cache_write_1h_per_1m_tokens"`
	FinalCacheReadPerMillion       *float64 `json:"final_cache_read_per_1m_tokens"`
	MaxInputTokens                 any      `json:"max_input_tokens,omitempty"`
	MaxOutputTokens                any      `json:"max_output_tokens,omitempty"`
}

type IntegrationSettings struct {
	Sub2APIEnabled           bool                           `json:"sub2api_enabled"`
	Sub2APIMainBaseURL       string                         `json:"sub2api_main_base_url"`
	Sub2APIAdminKey          string                         `json:"sub2api_admin_key,omitempty"`
	Sub2APIBaseURL           string                         `json:"sub2api_base_url,omitempty"`
	Sub2APIAccessToken       string                         `json:"sub2api_access_token,omitempty"`
	Sub2APIEmail             string                         `json:"sub2api_email"`
	Sub2APIPassword          string                         `json:"sub2api_password,omitempty"`
	Sub2APISyncAccountMode   string                         `json:"sub2api_sync_account_mode"`
	MonitorIntervalMinutes   int                            `json:"monitor_interval_minutes"`
	MonitorRuleDelaySeconds  int                            `json:"monitor_rule_delay_seconds"`
	LatencyTestEnabled       bool                           `json:"latency_test_enabled"`
	LatencyWeightPerSecond   float64                        `json:"latency_weight_per_second"`
	ExpectedCacheHitRatio    float64                        `json:"expected_cache_hit_ratio"`
	UpstreamBalanceThreshold float64                        `json:"upstream_balance_threshold"`
	SyncThresholdRatio       *float64                       `json:"sync_threshold_ratio"`
	SyncThresholdRatios      map[string]float64             `json:"sync_threshold_ratios"`
	EmailNotifyEnabled       bool                           `json:"email_notify_enabled"`
	EmailNotifyPriceChange   bool                           `json:"email_notify_price_change"`
	EmailNotifySyncUpdate    bool                           `json:"email_notify_sync_update"`
	SMTPHost                 string                         `json:"smtp_host"`
	SMTPPort                 int                            `json:"smtp_port"`
	SMTPEncryption           string                         `json:"smtp_encryption"`
	SMTPUsername             string                         `json:"smtp_username"`
	SMTPPassword             string                         `json:"smtp_password,omitempty"`
	SMTPFrom                 string                         `json:"smtp_from"`
	SMTPTo                   string                         `json:"smtp_to"`
	EmailTemplateEnabled     bool                           `json:"email_template_enabled"`
	EmailTemplateSubject     string                         `json:"email_template_subject"`
	EmailTemplateBody        string                         `json:"email_template_body"`
	EmailTemplateConfigs     map[string]EmailTemplateConfig `json:"email_template_configs"`
	UpdatedAt                time.Time                      `json:"updated_at"`
}

type EmailTemplateConfig struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}
