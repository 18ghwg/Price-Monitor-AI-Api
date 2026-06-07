package app

const indexHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>NewAPI 价格监控</title>
  <link rel="stylesheet" href="/static/app.css?v=20260607-hidden-fields">
</head>
<body>
  <section id="loginScreen" class="login-screen" hidden>
    <form id="loginForm" class="login-card">
      <span class="section-kicker">Admin</span>
      <h1>后台管理登录</h1>
      <p>输入部署时配置的管理员账号和密码。</p>
      <div id="loginError" class="login-error" role="alert" hidden></div>
      <label>账号<input name="username" required autocomplete="username" placeholder="请输入账号"></label>
      <label>密码<input name="password" type="password" required autocomplete="current-password" placeholder="请输入密码"></label>
      <button type="submit">登录</button>
    </form>
  </section>
  <main class="shell">
    <header class="hero">
      <nav class="topbar" aria-label="页面导航">
        <a class="brand" href="/">
          <span class="brand-mark" aria-hidden="true">PM</span>
          <span>
            <span class="brand-title">Price Monitor</span>
            <span class="brand-subtitle">NewAPI 价格监控</span>
          </span>
        </a>
        <div class="nav-tabs" role="tablist" aria-label="主视图">
          <button class="nav-tab active" data-app-view="monitor" type="button" role="tab" aria-selected="true">监控台</button>
          <button class="nav-tab" data-app-view="settings" type="button" role="tab" aria-selected="false">系统设置</button>
        </div>
        <div class="topbar-actions">
          <button id="refreshBtn" class="refresh-button" type="button">刷新数据</button>
          <button id="logoutBtn" class="secondary" type="button">退出</button>
        </div>
      </nav>

      <div class="hero-grid">
        <div class="hero-copy">
          <p class="eyebrow">NewAPI Operations Console</p>
          <h1>统一监控模型分组价格，及时发现成本变化</h1>
          <p class="hero-text">集中管理 NewAPI 和 sub2api 上游渠道、监控规则和价格快照，按完整模型名发现最低真实价格。</p>
        </div>
        <div class="hero-metrics" aria-label="监控概览">
          <div class="metric-card">
            <span class="metric-label">上游站点</span>
            <strong id="siteCount">0</strong>
          </div>
          <div class="metric-card">
            <span class="metric-label">监控规则</span>
            <strong id="ruleCount">0</strong>
          </div>
          <div class="metric-card">
            <span class="metric-label">价格快照</span>
            <strong id="snapshotCount">0</strong>
          </div>
          <div class="metric-card attention">
            <span class="metric-label">待排查</span>
            <strong id="issueCount">0</strong>
          </div>
        </div>
      </div>
    </header>

    <div id="monitorView" class="view" data-view-panel="monitor">
      <section class="workspace">
        <form id="siteForm" class="panel form-panel">
          <div class="panel-head">
            <span class="section-kicker">Step 01</span>
            <h2 id="siteFormTitle">添加上游站点账号</h2>
            <p id="siteFormHelp">用上方切换选择 NewAPI 或 sub2api，账号保存后会进入对应列表。</p>
          </div>
          <input name="id" type="hidden">
          <input name="source_type" id="siteSourceTypeInput" type="hidden" value="newapi">
          <div class="segmented-control" role="tablist" aria-label="上游账号类型">
            <button class="segment active" data-site-source-tab="newapi" type="button" role="tab" aria-selected="true">NewAPI</button>
            <button class="segment" data-site-source-tab="sub2api" type="button" role="tab" aria-selected="false">sub2api</button>
          </div>
          <label>站点名称<input name="name" required placeholder="NewAPI 上游 A"></label>
          <label>站点地址<input name="base_url" required autocomplete="url" placeholder="https://upstream.example.com"></label>
          <div class="form-grid">
            <label>用户名<input name="username" required autocomplete="username" placeholder="用户名或邮箱"></label>
            <label>密码<input name="password" type="password" required autocomplete="current-password"></label>
          </div>
          <label data-site-source-field="sub2api" hidden>Auth Token<input name="auth_token" type="password" disabled placeholder="可选，填写后不使用密码登录"></label>
          <label>2FA 当前验证码<input name="totp_code" autocomplete="one-time-code" placeholder="仅账号启用 2FA 时填写"></label>
          <div class="actions">
            <button id="siteSubmitBtn" type="submit">保存上游</button>
            <button id="siteCancelBtn" class="secondary" type="button" hidden>取消编辑</button>
          </div>
        </form>

        <form id="ruleForm" class="panel form-panel">
          <div class="panel-head">
            <span class="section-kicker">Step 02</span>
            <h2 id="ruleFormTitle">添加监控规则</h2>
            <p id="ruleFormHelp">选择 NewAPI 或 sub2api 上游渠道，按完整模型名监控指定模型，按真实价格优先排序并写入价格快照。</p>
          </div>
          <input name="id" type="hidden">
          <input name="source_type" id="ruleSourceTypeInput" type="hidden" value="newapi">
          <div class="segmented-control" role="tablist" aria-label="监控来源">
            <button class="segment active" data-rule-source-tab="newapi" type="button" role="tab" aria-selected="true">NewAPI</button>
            <button class="segment" data-rule-source-tab="sub2api" type="button" role="tab" aria-selected="false">sub2api</button>
          </div>
          <label data-rule-source-field="newapi">NewAPI 上游站点<select name="site_id" id="siteSelect"></select></label>
          <label data-rule-source-field="sub2api" hidden>sub2api 上游站点<select name="sub2api_upstream_id" id="ruleSub2UpstreamSelect" disabled></select></label>
          <label>分类
            <select name="category" id="ruleCategorySelect" required></select>
          </label>
          <label>完整模型名<input name="model_keyword" required placeholder="gpt-4o / claude-3-5-sonnet-20241022"></label>
          <div class="form-grid">
            <label>低价判断基准分组<input name="sync_base_group" placeholder="NewAPI 可填原分组名；留空则只记录最低价"></label>
            <label>定时快照间隔（分钟）<input name="interval_minutes" type="number" min="1" value="15"></label>
          </div>
          <div class="switch-row">
            <label class="checkbox-label"><input name="schedule_enabled" type="checkbox" checked>启用定时</label>
            <label class="checkbox-label"><input name="sync_enabled" type="checkbox">发现低价上游时同步到主站 sub2api</label>
          </div>
          <div class="actions">
            <button id="ruleSubmitBtn" type="submit">保存规则</button>
            <button id="ruleCancelBtn" class="secondary" type="button" hidden>取消编辑</button>
          </div>
        </form>
      </section>

      <section class="panel data-panel">
        <div class="panel-head row">
          <div>
            <span class="section-kicker">Rules</span>
            <h2>监控规则</h2>
            <p>启用定时后会按规则间隔自动写入价格快照；点击运行会立即登录站点并手动写入一次。</p>
          </div>
        </div>
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>ID</th>
                <th>站点</th>
                <th>分类</th>
                <th>关键词</th>
                <th>定时</th>
                <th>同步</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody id="rulesBody"></tbody>
          </table>
        </div>
      </section>

      <section class="panel data-panel">
        <div class="panel-head">
          <span class="section-kicker">Snapshots</span>
          <h2>最新价格快照</h2>
          <p>价格单位与原脚本一致：Token 类价格为 USD / 1M tokens，请求类价格为 USD / request。</p>
        </div>
        <div class="filters" aria-label="价格分类筛选">
          <div id="categoryFilters" class="filters-inner"></div>
        </div>
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th data-sort="created_at">时间</th>
                <th data-sort="site_name">站点</th>
                <th data-sort="category_name">分类</th>
                <th data-sort="model_keyword">模型名</th>
                <th data-sort="model_name">匹配模型</th>
                <th data-sort="group_name">最低价分组</th>
                <th data-sort="group_ratio">分组倍率</th>
                <th data-sort="upstream_balance">余额</th>
                <th data-sort="effective_price">最低有效价</th>
                <th data-sort="input_price">输入</th>
                <th data-sort="output_price">输出</th>
                <th data-sort="cache_read_price">缓存读</th>
                <th data-sort="cache_write_price">缓存写</th>
                <th data-sort="request_price">请求</th>
              </tr>
            </thead>
            <tbody id="snapshotsBody"></tbody>
          </table>
        </div>
      </section>

      <section class="panel data-panel">
        <div class="panel-head">
          <span class="section-kicker">Sites</span>
          <h2>站点状态</h2>
          <p>最近一次采集错误会显示在这里，便于排查登录或模型/分组名称问题。</p>
        </div>
        <div class="section-toolbar">
          <div class="segmented-control" role="tablist" aria-label="上游站点列表类型">
            <button class="segment active" data-site-list-tab="newapi" type="button" role="tab" aria-selected="true">NewAPI</button>
            <button class="segment" data-site-list-tab="sub2api" type="button" role="tab" aria-selected="false">sub2api</button>
          </div>
        </div>
        <div class="table-wrap compact-table">
          <table class="upstream-table">
            <thead>
              <tr>
                <th>ID</th>
                <th>站点</th>
                <th>地址</th>
                <th>用户名</th>
                <th>最近状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody id="sitesList"></tbody>
          </table>
        </div>
      </section>
    </div>

    <div id="settingsView" class="view" data-view-panel="settings" hidden>
      <section class="panel data-panel settings-page">
        <div class="panel-head settings-page-head">
          <span class="section-kicker">System Settings</span>
          <h2>系统设置</h2>
          <p>集中维护主站同步、邮件通知、监控分类和后台登录账号。</p>
        </div>

        <div class="settings-grid">
          <section class="settings-block wide">
            <div class="settings-block-head">
              <span class="section-kicker">Admin Account</span>
              <h3>后台登录账号</h3>
            </div>
            <form id="adminPasswordForm" class="settings-form">
              <div class="form-grid">
                <label>账号<input name="username" required autocomplete="username" placeholder="请输入账号"></label>
                <label>当前密码<input name="current_password" type="password" required autocomplete="current-password"></label>
              </div>
              <label>新密码<input name="new_password" type="password" required minlength="6" autocomplete="new-password" placeholder="至少 6 位"></label>
              <div class="actions">
                <button type="submit">修改登录密码</button>
              </div>
            </form>
          </section>

          <section class="settings-block wide">
            <div class="settings-block-head">
              <span class="section-kicker">Sub2API System</span>
              <h3>主站 sub2api 同步</h3>
            </div>
            <form id="settingsForm" class="settings-form">
              <div class="settings-note">
                <div>
                  <strong>同步写入开关</strong>
                  <p>这是价格监控系统写入主站 sub2api 的总开关，不在 sub2api 项目后台中设置。开启后，只有当前全局最低价快照才会创建或更新主站账号。</p>
                </div>
                <span id="sub2apiSyncStatus" class="sync-status off">未开启</span>
              </div>
              <div class="switch-row primary-switch">
                <label class="checkbox-label"><input name="sub2api_enabled" type="checkbox">启用主站 sub2api 同步</label>
              </div>
              <div class="form-grid">
                <label>主站 sub2api 地址<input name="sub2api_main_base_url" placeholder="https://sub2api.example.com"></label>
                <label>管理员 API Key<input name="sub2api_admin_key" type="password" placeholder="主站后台生成的 admin-...，留空保留现有值"></label>
              </div>
              <label>全局同步低价阈值倍率<input name="sync_threshold_ratio" type="number" min="0" step="0.000001" placeholder="例如 0.05，留空则不限制"></label>
              <div class="settings-subhead">
                <span class="section-kicker">Email Alerts</span>
                <h3>邮件通知</h3>
              </div>
              <div class="switch-row">
                <label class="checkbox-label"><input name="email_notify_enabled" type="checkbox">启用邮件通知</label>
                <label class="checkbox-label"><input name="email_notify_price_change" type="checkbox" checked>价格变动</label>
                <label class="checkbox-label"><input name="email_notify_sync_update" type="checkbox" checked>主站账号同步</label>
              </div>
              <div class="form-grid">
                <label>SMTP Host<input name="smtp_host" placeholder="smtp.example.com"></label>
                <label>SMTP Port<input name="smtp_port" type="number" min="1" value="587"></label>
                <label>SMTP 加密方式
                  <select name="smtp_encryption">
                    <option value="auto">自动</option>
                    <option value="ssl">SSL/TLS</option>
                    <option value="starttls">STARTTLS</option>
                    <option value="plain">不加密</option>
                  </select>
                </label>
              </div>
              <div class="form-grid">
                <label>SMTP 用户名<input name="smtp_username" autocomplete="username"></label>
                <label>SMTP 密码<input name="smtp_password" type="password" autocomplete="current-password" placeholder="留空表示保留现有密码"></label>
              </div>
              <div class="form-grid">
                <label>发件人<input name="smtp_from" placeholder="Price Monitor <alerts@example.com>"></label>
                <label>收件人<input name="smtp_to" placeholder="ops@example.com, admin@example.com"></label>
              </div>
              <div class="actions">
                <button type="submit">保存主站设置</button>
              </div>
            </form>
          </section>

          <section class="settings-block wide">
            <div class="settings-block-head">
              <span class="section-kicker">Sub2API Channels</span>
              <h3>主站渠道账号列表</h3>
            </div>
            <form id="sub2ChannelForm" class="settings-form">
              <div class="form-grid">
                <label>apiurl<input name="apiurl" placeholder="https://newapi.example.com"></label>
                <label>新 API Key<input name="api_key" type="password" placeholder="更新列表中指定账号时填写"></label>
              </div>
              <div class="actions">
                <button id="sub2SearchBtn" class="secondary" type="button">搜索渠道账号</button>
              </div>
            </form>
            <div class="table-wrap compact-table">
              <table>
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>账号</th>
                    <th>apiurl</th>
                    <th>分组</th>
                    <th>状态</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody id="sub2AccountsBody"></tbody>
              </table>
            </div>
          </section>

          <section class="settings-block wide">
            <div class="settings-block-head">
              <span class="section-kicker">User Pricing</span>
              <h3>sub2api 用户分组倍率和模型价格</h3>
            </div>
            <form id="sub2UserPriceForm" class="settings-form">
              <label>已保存上游 sub2api 站点<select name="sub2api_upstream_id" id="sub2UserPriceUpstreamSelect"></select></label>
              <label>站点地址<input name="base_url" placeholder="https://sub2api.example.com"></label>
              <div class="form-grid">
                <label>用户名<input name="email" autocomplete="username" placeholder="sub2api 登录邮箱或用户名"></label>
                <label>用户密码<input name="password" type="password" autocomplete="current-password"></label>
              </div>
              <div class="form-grid">
                <label>Auth Token<input name="auth_token" type="password" placeholder="可选，填写后不使用密码登录"></label>
                <label>2FA 验证码<input name="totp_code" placeholder="账号启用 2FA 时填写"></label>
              </div>
              <div class="form-grid">
                <label class="filter-field">模型关键词<input name="model_keyword" placeholder="codex / claude / gpt-4o"><span class="quick-options" data-fill-target="model_keyword"><button class="filter" type="button" data-fill-value="codex">Codex</button><button class="filter" type="button" data-fill-value="claude">Claude</button><button class="filter" type="button" data-fill-value="gpt">GPT</button></span></label>
                <label class="filter-field">平台过滤<input name="platforms" placeholder="openai,anthropic,gemini"><span class="quick-options" data-fill-target="platforms"><button class="filter" type="button" data-fill-value="openai">OpenAI</button><button class="filter" type="button" data-fill-value="anthropic">Claude</button><button class="filter" type="button" data-fill-value="gemini">Gemini</button></span></label>
              </div>
              <div class="form-grid">
                <label>模型过滤<input name="models" placeholder="精确模型名，多个用英文逗号分隔"></label>
                <label>分组过滤<input name="groups" placeholder="分组名称或 ID，多个用英文逗号分隔"></label>
              </div>
              <div class="form-grid">
                <label class="filter-field">Provider 过滤<input name="providers" placeholder="openai,anthropic"><span class="quick-options" data-fill-target="providers"><button class="filter" type="button" data-fill-value="openai">openai</button><button class="filter" type="button" data-fill-value="anthropic">anthropic</button><button class="filter" type="button" data-fill-value="google">google</button></span></label>
                <label class="filter-field">模式过滤<input name="modes" placeholder="chat,responses,image_generation"><span class="quick-options" data-fill-target="modes"><button class="filter" type="button" data-fill-value="chat">chat</button><button class="filter" type="button" data-fill-value="responses">responses</button><button class="filter" type="button" data-fill-value="image_generation">image</button></span></label>
              </div>
              <div class="form-grid">
                <label>返回上限<input name="limit" type="number" min="1" max="5000" value="500"></label>
              </div>
              <label>官方价格 JSON 地址<input name="price_url" placeholder="默认使用 Wei-Shaw/model-price-repo"></label>
              <div class="actions">
                <button id="sub2UserPriceBtn" type="button">获取分组倍率和模型价格</button>
              </div>
            </form>
            <div id="sub2UserFilterStatus" class="summary-line muted">选择已保存站点或填写站点登录信息后，会自动读取可选过滤项。</div>
            <div id="sub2UserFilterOptions" class="filter-options-panel"></div>
            <div id="sub2UserPriceSummary" class="summary-line muted">尚未获取用户价格。</div>
            <div id="sub2UserPriceControls" class="table-controls" hidden>
              <label class="table-search">模糊搜索<input id="sub2UserPriceSearch" type="search" placeholder="搜索模型、Provider、分组、平台"></label>
              <label class="table-page-size">每页<select id="sub2UserPricePageSize"><option value="10">10</option><option value="25" selected>25</option><option value="50">50</option><option value="100">100</option></select></label>
            </div>
            <div class="table-wrap compact-table">
              <table>
                <thead>
                  <tr>
                    <th>模型</th>
                    <th>Provider</th>
                    <th>分组</th>
                    <th>有效倍率</th>
                    <th>输入/1M</th>
                    <th>输出/1M</th>
                    <th>缓存写/1M</th>
                    <th>缓存读/1M</th>
                  </tr>
                </thead>
                <tbody id="sub2UserPricesBody"></tbody>
              </table>
            </div>
            <div id="sub2UserPricePager" class="pager" hidden></div>
          </section>

          <section class="settings-block wide">
            <div class="settings-block-head row">
              <div>
                <span class="section-kicker">Categories</span>
                <h3>监控分类</h3>
              </div>
              <form id="categoryForm" class="inline-form">
                <input name="id" type="hidden">
                <input name="name" required placeholder="分类名称">
                <input name="slug" placeholder="标识，可留空">
                <select id="categoryMainGroupSelect" aria-label="选择主站 sub2api 分组" multiple hidden>
                  <option value="">自动匹配或手动填写</option>
                </select>
                <div class="category-main-group-picker">
                  <div class="filter-option-title">主站 sub2api 分组</div>
                  <div id="categoryMainGroupChoices" class="category-main-group-choices"></div>
                  <div id="categoryMainGroupSummary" class="muted">未选择时按分类名称自动匹配。</div>
                </div>
                <input name="sub2api_main_group_name" placeholder="主站 sub2api 分组名">
                <input name="sub2api_main_group_id" type="number" min="0" value="0" placeholder="主站分组 ID">
                <button id="categorySubmitBtn" type="submit">保存分类</button>
                <button id="categoryCancelBtn" class="secondary" type="button" hidden>取消</button>
              </form>
            </div>
            <div id="categoriesList" class="category-list"></div>
          </section>
        </div>
      </section>
    </div>
    <footer class="app-footer">
      <span>开源地址</span>
      <a href="https://github.com/18ghwg/Price-Monitor-AI-Api.git" target="_blank" rel="noopener noreferrer">github.com/18ghwg/Price-Monitor-AI-Api</a>
    </footer>
  </main>
  <div id="toast" class="toast" hidden></div>
  <script src="/static/app.js?v=20260607-hidden-fields"></script>
</body>
</html>`

const appCSS = `:root {
  color-scheme: light;
  --bg: #f4f7fb;
  --bg-strong: #111827;
  --panel: rgba(255, 255, 255, .9);
  --panel-solid: #ffffff;
  --panel-soft: #f8fafc;
  --text: #111827;
  --muted: #667085;
  --muted-strong: #475467;
  --line: #dbe3ef;
  --line-strong: #cbd5e1;
  --primary: #8b5cf6;
  --primary-dark: #6d28d9;
  --accent: #f59e0b;
  --accent-soft: #fffbeb;
  --info: #0f766e;
  --info-soft: #ecfdf5;
  --danger: #b42318;
  --danger-soft: #fef2f2;
  --shadow: 0 18px 45px rgba(15, 23, 42, .09);
  --shadow-soft: 0 8px 24px rgba(15, 23, 42, .06);
  font-family: "Plus Jakarta Sans", Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

* { box-sizing: border-box; }

html {
  min-width: 320px;
  background: var(--bg);
}

body {
  margin: 0;
  min-width: 320px;
  background:
    radial-gradient(circle at 12% 0%, rgba(245, 158, 11, .2), transparent 30%),
    radial-gradient(circle at 92% 8%, rgba(139, 92, 246, .2), transparent 34%),
    linear-gradient(180deg, #f8fbff 0%, var(--bg) 42%, #eef3f8 100%);
  color: var(--text);
}

body.auth-pending .shell {
  display: none;
}

body, button, input, select {
  font: inherit;
}

button, input, select {
  min-width: 0;
}

button {
  min-height: 42px;
  border: 0;
  border-radius: 8px;
  padding: 0 16px;
  background: linear-gradient(135deg, var(--primary), var(--primary-dark));
  color: white;
  font-weight: 750;
  cursor: pointer;
  transition: background .18s ease, box-shadow .18s ease, transform .18s ease;
}

button:hover {
  box-shadow: 0 12px 22px rgba(109, 40, 217, .2);
  transform: translateY(-1px);
}

button:active {
  transform: translateY(0);
}

button:focus-visible,
input:focus-visible,
select:focus-visible,
a:focus-visible {
  outline: 3px solid rgba(139, 92, 246, .26);
  outline-offset: 2px;
}

button:disabled {
  cursor: wait;
  opacity: .68;
  transform: none;
}

button.secondary {
  border: 1px solid rgba(139, 92, 246, .22);
  background: #f5f3ff;
  color: var(--primary-dark);
}

button.secondary:hover {
  background: #ede9fe;
  box-shadow: var(--shadow-soft);
}

button.danger {
  border: 1px solid rgba(180, 35, 24, .18);
  background: var(--danger-soft);
  color: var(--danger);
}

button.danger:hover {
  background: #fee4e2;
  box-shadow: var(--shadow-soft);
}

h1, h2, p {
  margin-top: 0;
}

h1 {
  width: 100%;
  max-width: 780px;
  margin-bottom: 0;
  font-size: clamp(36px, 5vw, 64px);
  line-height: 1.02;
  letter-spacing: 0;
  overflow-wrap: break-word;
  word-break: break-word;
}

h2 {
  margin-bottom: 8px;
  font-size: 21px;
  line-height: 1.25;
  letter-spacing: 0;
}

h3 {
  margin: 0 0 8px;
  font-size: 17px;
  line-height: 1.3;
  letter-spacing: 0;
}

p {
  color: var(--muted);
  line-height: 1.65;
  overflow-wrap: anywhere;
}

a {
  color: inherit;
  text-decoration: none;
}

.shell {
  width: min(1320px, calc(100% - 40px));
  margin: 0 auto;
  padding: 24px 0 56px;
}

.hero {
  position: relative;
  overflow: hidden;
  margin-bottom: 22px;
  border: 1px solid rgba(255, 255, 255, .74);
  border-radius: 8px;
  background:
    linear-gradient(135deg, rgba(255, 255, 255, .96), rgba(255, 255, 255, .74)),
    linear-gradient(135deg, rgba(245, 158, 11, .16), rgba(139, 92, 246, .14));
  box-shadow: var(--shadow);
}

.hero::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background:
    linear-gradient(90deg, rgba(15, 23, 42, .04) 1px, transparent 1px),
    linear-gradient(180deg, rgba(15, 23, 42, .04) 1px, transparent 1px);
  background-size: 48px 48px;
  mask-image: linear-gradient(180deg, rgba(0, 0, 0, .65), transparent 78%);
}

.topbar {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 22px;
  border-bottom: 1px solid rgba(219, 227, 239, .72);
}

.brand {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.brand-mark {
  display: inline-grid;
  width: 42px;
  height: 42px;
  flex: 0 0 auto;
  place-items: center;
  border-radius: 8px;
  background: #111827;
  color: #f8fafc;
  font-size: 13px;
  font-weight: 850;
}

.brand-title,
.brand-subtitle {
  display: block;
  min-width: 0;
}

.brand-title {
  color: #111827;
  font-size: 15px;
  font-weight: 850;
}

.brand-subtitle {
  margin-top: 2px;
  color: var(--muted);
  font-size: 12px;
  font-weight: 650;
}

.refresh-button {
  flex: 0 0 auto;
}

.topbar-actions {
  display: flex;
  flex: 0 0 auto;
  flex-wrap: wrap;
  gap: 10px;
  justify-content: flex-end;
}

.nav-tabs {
  display: inline-flex;
  flex: 0 1 auto;
  gap: 4px;
  min-width: 0;
  padding: 4px;
  border: 1px solid rgba(219, 227, 239, .86);
  border-radius: 8px;
  background: rgba(255, 255, 255, .72);
}

button.nav-tab {
  min-height: 34px;
  padding: 0 12px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--muted-strong);
  box-shadow: none;
  font-size: 13px;
  font-weight: 850;
  white-space: nowrap;
}

button.nav-tab:hover {
  background: #f5f7fb;
  box-shadow: none;
  transform: none;
}

button.nav-tab.active,
button.nav-tab[aria-selected="true"] {
  background: #111827;
  color: #fff;
}

.hero-grid {
  position: relative;
  z-index: 1;
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(360px, .8fr);
  gap: 28px;
  padding: 34px 34px 38px;
}

.hero-copy {
  width: 100%;
  min-width: 0;
  align-self: center;
  overflow: hidden;
}

.eyebrow,
.section-kicker {
  margin: 0 0 10px;
  color: var(--primary-dark);
  font-size: 12px;
  font-weight: 850;
  letter-spacing: 0;
  text-transform: uppercase;
}

.hero-text {
  width: 100%;
  max-width: 700px;
  margin: 18px 0 0;
  color: var(--muted-strong);
  font-size: 17px;
}

.hero-metrics {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  min-width: 0;
}

.metric-card {
  min-width: 0;
  min-height: 132px;
  padding: 18px;
  border: 1px solid rgba(219, 227, 239, .82);
  border-radius: 8px;
  background: rgba(255, 255, 255, .72);
  box-shadow: var(--shadow-soft);
  backdrop-filter: blur(16px);
}

.metric-card.attention {
  background: linear-gradient(135deg, rgba(255, 251, 235, .9), rgba(255, 255, 255, .74));
}

.metric-label {
  display: block;
  color: var(--muted);
  font-size: 13px;
  font-weight: 750;
}

.metric-card strong {
  display: block;
  margin-top: 22px;
  font-size: 38px;
  line-height: 1;
  letter-spacing: 0;
}

.workspace {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  margin-bottom: 16px;
}

[hidden] {
  display: none !important;
}

.view[hidden] {
  display: none;
}

.panel {
  min-width: 0;
  margin-bottom: 16px;
  border: 1px solid rgba(219, 227, 239, .92);
  border-radius: 8px;
  background: var(--panel);
  box-shadow: var(--shadow-soft);
  backdrop-filter: blur(14px);
}

.form-panel {
  padding: 22px;
}

.data-panel {
  padding: 22px;
}

.panel-head {
  margin-bottom: 18px;
}

.panel-head.compact-head {
  margin-bottom: 12px;
}

.panel-head p {
  margin-bottom: 0;
}

.settings-page {
  padding: 24px;
}

.settings-page-head {
  max-width: 760px;
}

.settings-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.settings-block {
  min-width: 0;
  padding: 18px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: rgba(255, 255, 255, .72);
}

.settings-block.wide {
  grid-column: 1 / -1;
}

.settings-block-head {
  margin-bottom: 16px;
}

.settings-block-head h3,
.settings-subhead h3 {
  margin-bottom: 0;
}

.settings-subhead {
  margin-top: 6px;
  padding-top: 18px;
  border-top: 1px solid var(--line);
}

.settings-note {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 16px;
  border: 1px solid #c7d7fe;
  border-radius: 8px;
  background: #f8fbff;
  color: #24324a;
}

.settings-note strong {
  display: block;
  margin-bottom: 5px;
  font-size: 14px;
}

.settings-note p {
  margin: 0;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.6;
}

.primary-switch {
  margin-top: 2px;
}

.sync-status {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  min-height: 26px;
  padding: 0 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 800;
  white-space: nowrap;
}

.sync-status.on {
  border: 1px solid #a7f3d0;
  background: #ecfdf5;
  color: #047857;
}

.sync-status.off {
  border: 1px solid #fecaca;
  background: #fff7f7;
  color: #b42318;
}

.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

label {
  display: grid;
  gap: 8px;
  margin-bottom: 14px;
  min-width: 0;
  color: #344054;
  font-size: 14px;
  font-weight: 750;
}

input, select {
  width: 100%;
  height: 44px;
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 0 13px;
  background: #fff;
  color: var(--text);
  outline: none;
  transition: border-color .18s ease, box-shadow .18s ease, background .18s ease;
}

input[type="checkbox"] {
  width: 18px;
  height: 18px;
  padding: 0;
  accent-color: var(--primary-dark);
}

.switch-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 16px;
  margin: 4px 0 14px;
}

.section-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.segmented-control {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 4px;
  min-width: 0;
  padding: 4px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel-soft);
}

button.segment {
  min-height: 34px;
  width: auto;
  padding: 0 12px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--muted-strong);
  box-shadow: none;
  font-size: 13px;
  font-weight: 850;
}

button.segment:hover {
  background: #fff;
  box-shadow: none;
  transform: none;
}

button.segment.active,
button.segment[aria-selected="true"] {
  background: #111827;
  color: #fff;
}

.checkbox-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  margin: 0;
}

.settings-form {
  display: grid;
  gap: 12px;
}

.login-screen {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 24px;
}

.login-screen[hidden] {
  display: none;
}

.login-card {
  width: min(430px, 100%);
  padding: 26px;
  border: 1px solid rgba(219, 227, 239, .92);
  border-radius: 8px;
  background: var(--panel-solid);
  box-shadow: var(--shadow);
}

.login-card h1 {
  max-width: none;
  margin-bottom: 12px;
  font-size: 28px;
  line-height: 1.15;
}

.login-error {
  padding: 10px 12px;
  border: 1px solid #fecdd3;
  border-radius: 8px;
  margin-bottom: 14px;
  color: #be123c;
  background: #fff1f2;
  font-size: 14px;
  line-height: 1.45;
  overflow-wrap: break-word;
}

.login-error[hidden] {
  display: none;
}

input::placeholder {
  color: #98a2b3;
}

input:hover,
select:hover {
  border-color: var(--line-strong);
}

input[readonly] {
  background: #f2f4f7;
  color: var(--muted-strong);
  cursor: not-allowed;
}

input:focus,
select:focus {
  border-color: var(--primary);
  box-shadow: 0 0 0 4px rgba(139, 92, 246, .13);
}

.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 6px;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: -4px 0 16px;
}

.filters-inner {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  min-width: 0;
}

button.filter {
  min-height: 36px;
  border: 1px solid var(--line);
  background: #fff;
  color: var(--muted-strong);
  box-shadow: none;
}

button.filter:hover,
button.filter.active {
  border-color: rgba(139, 92, 246, .3);
  background: #f5f3ff;
  color: var(--primary-dark);
  box-shadow: var(--shadow-soft);
}

.filter-field {
  align-content: start;
}

.quick-options {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-width: 0;
}

.quick-options button.filter {
  min-height: 30px;
  padding: 0 10px;
  font-size: 12px;
}

.filter-options-panel {
  display: grid;
  gap: 12px;
  margin: 10px 0 12px;
}

.filter-option-group {
  display: grid;
  gap: 8px;
}

.filter-option-title {
  color: #344054;
  font-size: 13px;
  font-weight: 850;
}

.filter-option-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-width: 0;
}

.filter-option-buttons button.filter {
  min-height: 30px;
  padding: 0 10px;
  font-size: 12px;
}

.category-tag {
  display: inline-flex;
  align-items: center;
  min-height: 26px;
  padding: 0 9px;
  border-radius: 999px;
  background: #eef2ff;
  color: #4338ca;
  font-size: 12px;
  font-weight: 850;
}

.category-tag.claud {
  background: #fff7ed;
  color: #c2410c;
}

.category-tag.other {
  background: #f2f4f7;
  color: #475467;
}

.group-badge {
  display: inline-flex;
  align-items: center;
  min-height: 26px;
  padding: 0 9px;
  border-radius: 999px;
  background: #ecfdf5;
  color: #047857;
  font-size: 12px;
  font-weight: 850;
}

.source-badge {
  display: inline-flex;
  align-items: center;
  flex: 0 0 auto;
  min-height: 26px;
  padding: 0 9px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 850;
}

.source-badge.newapi {
  background: #eef2ff;
  color: #4338ca;
}

.source-badge.sub2api {
  background: #fff7ed;
  color: #c2410c;
}

.source-badge.invalid {
  margin-left: 6px;
  background: #f2f4f7;
  color: #667085;
}

.source-site {
  display: inline-flex;
  align-items: center;
  flex-wrap: nowrap;
  gap: 8px;
  min-width: 0;
  max-width: 100%;
  white-space: nowrap;
}

.source-site .site-link,
.source-site-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.inline-form {
  display: grid;
  grid-template-columns: minmax(120px, 1fr) minmax(120px, 1fr) minmax(260px, 1.6fr) minmax(120px, 1fr) minmax(90px, .7fr) auto auto;
  gap: 10px;
  min-width: min(100%, 960px);
}

.inline-form input,
.inline-form select {
  height: 42px;
  min-width: 0;
}

.inline-form select[multiple] {
  height: 96px;
}

.category-main-group-picker {
  display: grid;
  gap: 8px;
  min-width: 0;
  padding: 10px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel-soft);
}

.category-main-group-choices {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-width: 0;
  max-height: 118px;
  overflow: auto;
}

.category-main-group-choice {
  width: auto;
  min-height: 30px;
  min-width: 0;
  padding: 0 10px;
  border-color: #cbd5e1;
  background: #fff;
  color: #344054;
  font-size: 12px;
  overflow-wrap: anywhere;
}

.category-main-group-choice.active {
  border-color: rgba(109, 40, 217, .32);
  background: #eef2ff;
  color: #4338ca;
}

.category-main-group-choice[data-platform*="anthropic"].active,
.category-main-group-choice[data-platform*="claude"].active {
  border-color: rgba(194, 65, 12, .28);
  background: #fff7ed;
  color: #c2410c;
}

.category-list {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.category-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  max-width: 100%;
  min-height: 40px;
  padding: 6px 8px 6px 12px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: #fff;
}

.category-chip strong,
.site-link {
  min-width: 0;
  overflow-wrap: anywhere;
}

.category-actions {
  display: inline-flex;
  gap: 6px;
}

.category-main-group-tags {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 5px;
  min-width: 0;
}

.panel-divider {
  height: 1px;
  margin: 24px 0;
  background: var(--line);
}

.summary-line {
  margin: 14px 0 10px;
}

.cheapest-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px;
  margin: 12px 0 4px;
}

.cheapest-card {
  display: grid;
  gap: 8px;
  min-width: 0;
  padding: 14px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel-soft);
}

.cheapest-card strong,
.cheapest-card span {
  min-width: 0;
  overflow-wrap: anywhere;
}

.cheapest-rate {
  color: var(--info);
  font-size: 22px;
  font-weight: 900;
}

.category-actions button,
.table-actions button {
  min-height: 30px;
  padding: 0 9px;
  font-size: 12px;
}

.table-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-width: 180px;
}

.site-link {
  color: var(--primary-dark);
  font-weight: 850;
  text-decoration: underline;
  text-decoration-color: rgba(109, 40, 217, .28);
  text-underline-offset: 3px;
}

.site-link:hover {
  color: #4c1d95;
}

.table-wrap {
  overflow-x: auto;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel-solid);
}

table {
  width: 100%;
  min-width: 980px;
  border-collapse: collapse;
}

.compact-table {
  margin-top: 14px;
}

.table-controls,
.pager {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin: 10px 0;
}

.table-controls[hidden],
.pager[hidden] {
  display: none;
}

.table-search {
  flex: 1 1 260px;
  min-width: 0;
}

.table-page-size {
  width: 150px;
}

.pager {
  justify-content: flex-end;
}

.pager-info {
  color: var(--muted);
  font-size: 13px;
}

.pager button {
  width: auto;
  min-height: 32px;
  padding: 0 10px;
}

.compact-table table {
  min-width: 760px;
}

th, td {
  padding: 14px 16px;
  border-bottom: 1px solid #edf1f7;
  text-align: left;
  vertical-align: top;
  overflow-wrap: break-word;
}

th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: #f8fafc;
  color: #667085;
  font-size: 12px;
  font-weight: 850;
  text-transform: uppercase;
  letter-spacing: 0;
}

th[data-sort] {
  cursor: pointer;
  user-select: none;
}

th[data-sort]::after {
  content: "↕";
  display: inline-block;
  margin-left: 6px;
  color: #98a2b3;
  font-size: 11px;
}

th[data-sort].sorted-asc::after {
  content: "↑";
  color: var(--primary-dark);
}

th[data-sort].sorted-desc::after {
  content: "↓";
  color: var(--primary-dark);
}

td {
  color: #344054;
  font-size: 14px;
}

tbody tr {
  transition: background .16s ease;
}

tbody tr:hover {
  background: #fbfdff;
}

tr:last-child td {
  border-bottom: 0;
}

.status {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  padding: 0 10px;
  border: 1px solid rgba(15, 118, 110, .16);
  border-radius: 999px;
  background: var(--info-soft);
  color: var(--info);
  font-size: 12px;
  font-weight: 850;
}

.status.disabled {
  border-color: rgba(102, 112, 133, .18);
  background: #f2f4f7;
  color: var(--muted);
}

.site-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.site-item {
  display: grid;
  gap: 8px;
  min-width: 0;
  padding: 16px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: linear-gradient(180deg, #ffffff, #f9fbfd);
}

.site-title {
  min-width: 0;
  color: var(--text);
  font-weight: 850;
  overflow-wrap: break-word;
}

.site-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.site-actions {
  display: flex;
  flex: 0 0 auto;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-end;
}

.muted {
  color: var(--muted);
  overflow-wrap: break-word;
}

.error {
  padding: 10px 12px;
  border: 1px solid rgba(180, 35, 24, .16);
  border-radius: 8px;
  background: var(--danger-soft);
  color: var(--danger);
  overflow-wrap: break-word;
}

.empty-state {
  padding: 22px;
  color: var(--muted);
  text-align: center;
}

.app-footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin-top: 22px;
  padding: 16px 12px 0;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.5;
}

.app-footer a {
  color: var(--primary-dark);
  font-weight: 800;
  text-decoration: underline;
  text-decoration-color: rgba(109, 40, 217, .28);
  text-underline-offset: 3px;
  overflow-wrap: anywhere;
}

.toast {
  position: fixed;
  right: 20px;
  bottom: 20px;
  z-index: 20;
  max-width: min(420px, calc(100vw - 40px));
  padding: 14px 16px;
  border: 1px solid rgba(255, 255, 255, .12);
  border-radius: 8px;
  background: #111827;
  color: white;
  box-shadow: 0 18px 50px rgba(15, 23, 42, .28);
  overflow-wrap: break-word;
}

@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    scroll-behavior: auto !important;
    transition-duration: .01ms !important;
    animation-duration: .01ms !important;
  }
}

@media (max-width: 980px) {
  .hero-grid,
  .workspace,
  .settings-grid,
  .site-list {
    grid-template-columns: 1fr;
  }

  .hero-grid {
    gap: 22px;
  }
}

@media (max-width: 760px) {
  .shell {
    width: min(100% - 24px, 1320px);
    padding-top: 12px;
    padding-bottom: 36px;
  }

  .topbar,
  .row {
    align-items: stretch;
    flex-direction: column;
  }

  .nav-tabs {
    width: 100%;
  }

  button.nav-tab {
    flex: 1 1 0;
  }

  .topbar-actions {
    justify-content: stretch;
  }

  .settings-note {
    flex-direction: column;
    align-items: stretch;
  }

  .sync-status {
    width: fit-content;
  }

  .topbar-actions button {
    flex: 1 1 130px;
  }

  .hero-grid {
    padding: 24px 18px 22px;
    min-width: 0;
  }

  h1 {
    max-width: 11em;
    font-size: 28px;
    line-height: 1.14;
    word-break: break-all;
  }

  .hero-metrics,
  .form-grid {
    grid-template-columns: 1fr;
  }

  .hero-text {
    max-width: 22em;
    font-size: 14px;
    line-height: 1.7;
  }

  .metric-card {
    min-height: 104px;
  }

  .metric-card strong {
    margin-top: 14px;
    font-size: 32px;
  }

  .form-panel,
  .data-panel,
  .settings-page,
  .settings-block {
    padding: 16px;
  }

  .site-header {
    align-items: stretch;
    flex-direction: column;
  }

  .site-actions,
  .actions,
  .category-actions {
    justify-content: flex-start;
  }

  .inline-form {
    grid-template-columns: 1fr;
  }

  button {
    width: 100%;
  }

  button.nav-tab,
  button.segment {
    width: auto;
  }
}`

const appJS = `const state = {
  sites: [],
  categories: [],
  rules: [],
  snapshots: [],
  sub2Upstreams: [],
  sub2Accounts: [],
  sub2Groups: [],
  sub2UserPrices: null,
  sub2UserPricePage: 1,
  sub2UserPricePageSize: 25,
  sub2UserPriceSearch: "",
  sub2UserFilterOptions: null,
  sub2UserFilterLoading: false,
  sub2Inspect: null,
  settings: null,
  editingSiteId: null,
  editingSub2UpstreamId: null,
  editingCategoryId: null,
  editingRuleId: null,
  activeView: "monitor",
  activeSiteSourceType: "newapi",
  activeSiteListType: "newapi",
  activeRuleSourceType: "newapi",
  categoryFilter: "all",
  sort: { key: "effective_price", dir: "asc" },
};

let sub2UserFilterTimer = null;
let sub2UserFilterRequestId = 0;

const $ = (selector) => document.querySelector(selector);
document.body.classList.add("auth-pending");

async function api(path, options = {}) {
  const url = new URL(path, window.location.origin);
  const response = await fetch(url.toString(), {
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    if (response.status === 401) showLogin();
    throw new Error(payload?.error?.message || "请求失败");
  }
  return payload.data;
}

function formJSON(form) {
  const data = new FormData(form);
  const payload = Object.fromEntries(data.entries());
  form.querySelectorAll("input[type=checkbox]").forEach((input) => {
    payload[input.name] = input.checked;
  });
  return payload;
}

function fmt(value) {
  if (value === null || value === undefined || value === "") return "";
  if (typeof value === "number") return Number.isInteger(value) ? String(value) : value.toPrecision(8);
  return String(value);
}

function fmtBalance(value, unit) {
  if (value === null || value === undefined || value === "") return "未知";
  const numeric = Number(value);
  const normalized = String(unit || "").trim().toLowerCase();
  if (normalized === "usd") {
    return Number.isFinite(numeric) ? "$" + numeric.toFixed(6) : fmt(value) + " USD";
  }
  if (normalized === "quota") {
    return Number.isFinite(numeric) ? "$" + (numeric / 500000).toFixed(6) : fmt(value) + " quota";
  }
  if (normalized === "balance") {
    return Number.isFinite(numeric) ? "$" + numeric.toFixed(6) : fmt(value);
  }
  const label = fmt(value);
  return normalized ? label + " " + normalized : label;
}

function fmtTime(value) {
  if (!value) return "";
  return new Date(value).toLocaleString();
}

function toast(message) {
  const box = $("#toast");
  if (!box) return;
  box.textContent = message;
  box.hidden = false;
  clearTimeout(window.__toastTimer);
  window.__toastTimer = setTimeout(() => { box.hidden = true; }, 3200);
}

function setLoginError(message) {
  const box = $("#loginError");
  if (!box) return;
  box.textContent = message || "";
  box.hidden = !message;
}

function setText(selector, value) {
  const node = $(selector);
  if (node) node.textContent = value;
}

async function loadAll() {
  const [sites, categories, rules, snapshots, settings, sub2Upstreams, sub2Groups] = await Promise.all([
    api("/api/sites"),
    api("/api/categories"),
    api("/api/rules"),
    api("/api/snapshots?limit=500&category=" + encodeURIComponent(state.categoryFilter)),
    api("/api/settings"),
    api("/api/sub2api/upstreams"),
    api("/api/sub2api/groups").catch(() => []),
  ]);
  state.sites = sites || [];
  state.categories = categories || [];
  state.rules = rules || [];
  state.snapshots = snapshots || [];
  state.settings = settings || null;
  state.sub2Upstreams = sub2Upstreams || [];
  state.sub2Groups = sub2Groups || [];
  if (state.categoryFilter !== "all" && !state.categories.some((item) => item.slug === state.categoryFilter)) {
    state.categoryFilter = "all";
    state.snapshots = await api("/api/snapshots?limit=500&category=all") || [];
  }
  render();
}

function render() {
  const issueCount = state.sites.filter((site) => site.last_error).length;
  setText("#siteCount", state.sites.length);
  setText("#ruleCount", state.rules.length);
  setText("#snapshotCount", state.snapshots.length);
  setText("#issueCount", issueCount);

  renderCategoryControls();
  renderCategoryMainGroupOptions();
  renderRuleFormOptions();
  renderRules();
  renderSnapshots();
  renderSites();
  renderSettings();
  renderSub2Accounts();
  renderSub2UserPrices();
  renderSub2UserFilterOptions();
  renderSortHeaders();
  renderView();
}

function newapiSites() {
  return state.sites.filter((site) => String(site.source_type || "newapi").toLowerCase() !== "sub2api");
}

function sub2Sites() {
  return state.sites.filter((site) => String(site.source_type || "").toLowerCase() === "sub2api");
}

function setActiveView(view) {
  state.activeView = view === "settings" ? "settings" : "monitor";
  renderView();
}

function renderView() {
  document.querySelectorAll("[data-view-panel]").forEach((panel) => {
    panel.hidden = panel.getAttribute("data-view-panel") !== state.activeView;
  });
  document.querySelectorAll("[data-app-view]").forEach((button) => {
    const active = button.getAttribute("data-app-view") === state.activeView;
    button.classList.toggle("active", active);
    button.setAttribute("aria-selected", active ? "true" : "false");
  });
}

async function boot() {
  try {
    const session = await api("/api/auth/session");
    if (!session?.authenticated) {
      showLogin();
      return;
    }
    hideLogin();
    await loadAll();
  } catch (error) {
    showLogin();
  }
}

function showLogin() {
  const screen = $("#loginScreen");
  if (screen) screen.hidden = false;
  document.body.classList.add("auth-pending");
}

function hideLogin() {
  const screen = $("#loginScreen");
  if (screen) screen.hidden = true;
  setLoginError("");
  document.body.classList.remove("auth-pending");
}

function renderCategoryControls() {
  const filters = $("#categoryFilters");
  if (filters) {
    const buttons = [{ slug: "all", name: "全部" }, ...state.categories].map((category) => {
      const active = category.slug === state.categoryFilter ? " active" : "";
      return "<button class=\"filter" + active + "\" data-category-filter=\"" + escapeAttr(category.slug) + "\" type=\"button\">" + escapeHTML(category.name) + "</button>";
    });
    filters.innerHTML = buttons.join("");
  }

  const list = $("#categoriesList");
  if (!list) return;
  list.innerHTML = state.categories.length ? state.categories.map((category) => {
    const protectedDelete = category.slug === "other" ? " disabled" : "";
    const mainGroups = categoryMainGroups(category);
    const mainGroup = mainGroups.length
      ? "<span class=\"category-main-group-tags\">" + mainGroups.map((group) => "<span class=\"group-badge\">" + escapeHTML((group.name || "未命名") + (group.id ? " #" + group.id : "")) + "</span>").join("") + "</span>"
      : "<span class=\"muted\">主站分组：按分类名称自动匹配</span>";
    return "<span class=\"category-chip\">"
      + "<strong>" + escapeHTML(category.name) + "</strong>"
      + "<span class=\"muted\">" + escapeHTML(category.slug) + "</span>"
      + mainGroup
      + "<span class=\"category-actions\">"
      + "<button class=\"secondary\" data-edit-category=\"" + category.id + "\" type=\"button\">编辑</button>"
      + "<button class=\"danger\" data-delete-category=\"" + category.id + "\" type=\"button\"" + protectedDelete + ">删除</button>"
      + "</span>"
      + "</span>";
  }).join("") : "<div class=\"empty-state\">暂无分类。</div>";
}

function renderCategoryMainGroupOptions() {
  const select = $("#categoryMainGroupSelect");
  const choices = $("#categoryMainGroupChoices");
  const summary = $("#categoryMainGroupSummary");
  if (!select) return;
  const selectedIDs = new Set(selectedCategoryMainGroupIDs());
  const groups = categoryMainGroupChoices();
  select.innerHTML = groups.map((group) => {
      const label = categoryMainGroupLabel(group);
      return "<option value=\"" + escapeAttr(group.id) + "\">" + escapeHTML(label) + "</option>";
    }).join("");
  Array.from(select.options).forEach((option) => {
    option.selected = selectedIDs.has(String(option.value));
  });
  if (choices) {
    choices.innerHTML = groups.length ? groups.map((group) => {
      const id = String(group.id || "");
      const platform = String(group.platform || "");
      const active = selectedIDs.has(id) ? " active" : "";
      return "<button class=\"category-main-group-choice" + active + "\" type=\"button\" data-category-main-group=\"" + escapeAttr(id) + "\" data-platform=\"" + escapeAttr(platform) + "\">"
        + escapeHTML(categoryMainGroupLabel(group))
        + "</button>";
    }).join("") : "<span class=\"muted\">还没有读取到主站 sub2api 分组，请先检查系统设置里的主站地址和管理员 key。</span>";
  }
  if (summary) {
    updateCategoryMainGroupSummary();
  }
  select.title = "可点击上方分组按钮多选，也可手动填写名称和 ID";
}

function applyCategoryMainGroupSelection() {
  const form = $("#categoryForm");
  const select = $("#categoryMainGroupSelect");
  if (!form || !select) return;
  const groups = selectedCategoryMainGroups();
  const first = groups[0] || {};
  form.elements.sub2api_main_group_id.value = String(first.id || 0);
  form.elements.sub2api_main_group_name.value = groups.map((group) => group.name || ("#" + group.id)).filter(Boolean).join(",");
  form.dataset.mainGroupIds = groups.map((group) => String(group.id)).join(",");
  form.dataset.mainGroupsJson = JSON.stringify(groups);
  syncCategoryMainGroupChoiceState();
  updateCategoryMainGroupSummary();
}

function categoryMainGroupChoices() {
  const byID = new Map();
  state.sub2Groups.forEach((group) => {
    if (group?.id) byID.set(String(group.id), group);
  });
  const form = $("#categoryForm");
  if (form?.dataset.mainGroupsJson) {
    try {
      const selected = JSON.parse(form.dataset.mainGroupsJson);
      if (Array.isArray(selected)) {
        selected.forEach((group) => {
          if (group?.id && !byID.has(String(group.id))) byID.set(String(group.id), group);
        });
      }
    } catch (_error) {}
  }
  return Array.from(byID.values());
}

function categoryMainGroupLabel(group) {
  return (group?.name || "未命名")
    + (group?.id ? " #" + group.id : "")
    + " · " + (group?.platform || "unknown")
    + (group?.rate_multiplier || group?.rate ? " · " + fmt(group.rate_multiplier || group.rate) : "");
}

function syncCategoryMainGroupChoiceState() {
  const selectedIDs = new Set(selectedCategoryMainGroupIDs());
  document.querySelectorAll("[data-category-main-group]").forEach((button) => {
    button.classList.toggle("active", selectedIDs.has(String(button.getAttribute("data-category-main-group"))));
  });
}

function updateCategoryMainGroupSummary() {
  const summary = $("#categoryMainGroupSummary");
  if (!summary) return;
  const groups = selectedCategoryMainGroups();
  summary.textContent = groups.length
    ? "已选择 " + groups.length + " 个主站分组：" + groups.map((group) => (group.name || "未命名") + (group.id ? " #" + group.id : "")).join("，")
    : "未选择时按分类名称自动匹配。";
}

function toggleCategoryMainGroup(groupID) {
  const select = $("#categoryMainGroupSelect");
  if (!select || !groupID) return;
  const option = Array.from(select.options).find((item) => String(item.value) === String(groupID));
  if (!option) return;
  option.selected = !option.selected;
  applyCategoryMainGroupSelection();
}

async function refreshSub2MainGroups() {
  state.sub2Groups = await api("/api/sub2api/groups").catch((error) => {
    toast(error.message);
    return state.sub2Groups || [];
  }) || [];
  renderCategoryMainGroupOptions();
  applyCategoryMainGroupSelection();
}

function categoryMainGroups(category) {
  const groups = Array.isArray(category?.sub2api_main_groups) ? category.sub2api_main_groups : [];
  if (groups.length) return groups.filter((group) => group && (group.id || group.name));
  if (category?.sub2api_main_group_id || category?.sub2api_main_group_name) {
    return [{ id: category.sub2api_main_group_id || 0, name: category.sub2api_main_group_name || "" }];
  }
  return [];
}

function selectedCategoryMainGroupIDs() {
  const form = $("#categoryForm");
  const explicit = form?.dataset.mainGroupIds || "";
  if (explicit) return explicit.split(",").map((id) => id.trim()).filter(Boolean);
  const select = $("#categoryMainGroupSelect");
  const selected = select ? Array.from(select.selectedOptions).map((option) => String(option.value)).filter(Boolean) : [];
  if (selected.length) return selected;
  if (select && select.options.length > 0) return [];
  const currentID = form?.elements.sub2api_main_group_id?.value || "0";
  return currentID && currentID !== "0" ? [currentID] : [];
}

function selectedCategoryMainGroups() {
  const select = $("#categoryMainGroupSelect");
  const choices = categoryMainGroupChoices();
  const selected = select ? Array.from(select.selectedOptions)
    .map((option) => choices.find((group) => String(group.id) === String(option.value)))
    .filter(Boolean)
    .map((group) => ({ id: Number(group.id || 0), name: group.name || "" })) : [];
  if (select && select.options.length > 0) return selected;
  if (selected.length) return selected;
  const form = $("#categoryForm");
  if (!form?.dataset.mainGroupsJson) return [];
  try {
    const groups = JSON.parse(form.dataset.mainGroupsJson);
    return Array.isArray(groups) ? groups.filter((group) => group && (group.id || group.name)) : [];
  } catch (_error) {
    return [];
  }
}

function renderRuleFormOptions() {
  const siteSelect = $("#siteSelect");
  if (siteSelect) {
    const selected = siteSelect.value || "";
    const sites = newapiSites();
    siteSelect.innerHTML = sites.length
      ? sites.map((site) => "<option value=\"" + site.id + "\">" + escapeHTML(site.name) + " · " + escapeHTML(site.base_url) + "</option>").join("")
      : "<option value=\"\" selected disabled>请先添加 NewAPI 上游</option>";
    if (sites.some((site) => String(site.id) === selected)) {
      siteSelect.value = selected;
    }
  }

  const categorySelect = $("#ruleCategorySelect");
  if (categorySelect) {
    categorySelect.innerHTML = state.categories.length
      ? state.categories.map((category) => "<option value=\"" + escapeAttr(category.slug) + "\">" + escapeHTML(category.name) + "</option>").join("")
      : "<option value=\"other\">其他</option>";
  }

  const upstreamSelect = $("#ruleSub2UpstreamSelect");
  if (upstreamSelect) {
    const selected = upstreamSelect.value || "0";
    upstreamSelect.innerHTML = "<option value=\"0\">请选择 sub2api 上游站点</option>"
      + state.sub2Upstreams.map((upstream) => "<option value=\"" + upstream.id + "\">" + escapeHTML(upstream.name) + " · " + escapeHTML(upstream.base_url) + "</option>").join("");
    if (state.sub2Upstreams.some((upstream) => String(upstream.id) === selected)) {
      upstreamSelect.value = selected;
    } else if (currentRuleSourceType() === "sub2api" && state.sub2Upstreams.length) {
      upstreamSelect.value = String(state.sub2Upstreams[0].id);
    } else {
      upstreamSelect.value = "0";
    }
  }

  const priceUpstreamSelect = $("#sub2UserPriceUpstreamSelect");
  if (priceUpstreamSelect) {
    const selected = priceUpstreamSelect.value || "0";
    priceUpstreamSelect.innerHTML = "<option value=\"0\">手动填写账号</option>"
      + state.sub2Upstreams.map((upstream) => "<option value=\"" + upstream.id + "\">" + escapeHTML(upstream.name) + " · " + escapeHTML(upstream.base_url) + "</option>").join("");
    priceUpstreamSelect.value = state.sub2Upstreams.some((upstream) => String(upstream.id) === selected) ? selected : "0";
    syncSub2UserPriceUpstreamFields();
    if (priceUpstreamSelect.value !== "0" && !state.sub2UserFilterOptions && !state.sub2UserFilterLoading) {
      scheduleSub2UserFilterOptionsLoad();
    }
  }
  syncRuleSourceFields();
}

function currentRuleSourceType() {
  return state.activeRuleSourceType === "sub2api" ? "sub2api" : "newapi";
}

function setRuleSourceType(sourceType) {
  state.activeRuleSourceType = sourceType === "sub2api" ? "sub2api" : "newapi";
  const input = $("#ruleSourceTypeInput");
  if (input) input.value = state.activeRuleSourceType;
  syncRuleSourceFields();
}

function renderRuleSourceTabs() {
  document.querySelectorAll("[data-rule-source-tab]").forEach((button) => {
    const active = button.getAttribute("data-rule-source-tab") === state.activeRuleSourceType;
    button.classList.toggle("active", active);
    button.setAttribute("aria-selected", active ? "true" : "false");
  });
}

function syncRuleSourceFields() {
  const sourceType = currentRuleSourceType();
  renderRuleSourceTabs();
  const form = $("#ruleForm");
  if (form?.elements.source_type) form.elements.source_type.value = sourceType;
  document.querySelectorAll("[data-rule-source-field]").forEach((field) => {
    const active = field.getAttribute("data-rule-source-field") === sourceType;
    field.hidden = !active;
    field.querySelectorAll("select,input").forEach((input) => {
      input.disabled = !active;
      input.required = active && (
        (input.name === "site_id" && sourceType === "newapi")
        || (input.name === "sub2api_upstream_id" && sourceType === "sub2api")
      );
    });
  });
  ensureRuleSourceSelection(sourceType);
}

function ensureRuleSourceSelection(sourceType) {
  const siteSelect = $("#siteSelect");
  const upstreamSelect = $("#ruleSub2UpstreamSelect");
  if (sourceType === "sub2api") {
    if (upstreamSelect && (!Number(upstreamSelect.value || 0)) && state.sub2Upstreams.length) {
      upstreamSelect.value = String(state.sub2Upstreams[0].id);
    }
    return;
  }
  if (siteSelect && (!Number(siteSelect.value || 0)) && state.sites.length) {
    const sites = newapiSites();
    if (sites.length) siteSelect.value = String(sites[0].id);
  }
}

function renderRules() {
  const body = $("#rulesBody");
  if (!body) return;
  body.innerHTML = state.rules.length ? state.rules.map((rule) => {
    const id = rule.id;
    const isEnabled = rule.enabled;
    const enabled = isEnabled ? "启用" : "停用";
    const statusClass = isEnabled ? "status" : "status disabled";
    const schedule = rule.schedule_enabled ? ("每 " + (rule.interval_minutes || 15) + " 分钟") : "关闭";
    const sync = rule.sync_enabled
      ? ("<span class=\"status\">" + escapeHTML(rule.sync_status || "待同步") + "</span>" + (rule.sync_error ? "<div class=\"error\">" + escapeHTML(rule.sync_error) + "</div>" : ""))
      : "<span class=\"status disabled\">关闭</span>";
    return "<tr>"
      + "<td>" + id + "</td>"
      + "<td><span class=\"source-site\">" + sourceBadge(rule.source_type) + "<span class=\"source-site-name\">" + escapeHTML(rule.source_name || rule.site_name || "") + "</span></span></td>"
      + "<td>" + categoryTag(rule.category, rule.category_name) + "</td>"
      + "<td>" + escapeHTML(rule.model_keyword || rule.model_name || "") + "<div class=\"muted\">基准：" + escapeHTML(rule.sync_base_group || rule.group_name || "未设置") + "</div></td>"
      + "<td>" + escapeHTML(schedule)
      + "<div class=\"muted\">上次：" + (fmtTime(rule.last_scheduled_run_at) || "尚未定时运行") + "</div>"
      + "<div class=\"muted\">下次：" + (fmtTime(rule.next_run_at) || "未排期") + "</div></td>"
      + "<td>" + sync + "<div class=\"muted\">主站分组按分类绑定</div></td>"
      + "<td><span class=\"" + statusClass + "\">" + enabled + "</span></td>"
      + "<td><div class=\"table-actions\">"
      + "<button class=\"secondary\" data-edit-rule=\"" + id + "\" type=\"button\">编辑</button>"
      + "<button class=\"danger\" data-delete-rule=\"" + id + "\" type=\"button\">删除</button>"
      + "<button class=\"secondary\" data-run=\"" + id + "\" type=\"button\">运行一次</button>"
      + "</div></td>"
      + "</tr>";
  }).join("") : "<tr><td class=\"empty-state\" colspan=\"8\">暂无监控规则，添加站点后创建第一条关键词监控。</td></tr>";
}

function renderSnapshots() {
  const body = $("#snapshotsBody");
  if (!body) return;
  const rows = sortedSnapshots();
  body.innerHTML = rows.length ? rows.map((row) => {
    const invalidBadge = row.invalid ? "<span class=\"source-badge invalid\">失效</span>" : "";
    const invalidReason = row.invalid ? "<div class=\"muted\">失效：" + escapeHTML(row.invalid_reason || "上游分组已不存在") + "</div>" : "";
    return "<tr>"
      + "<td>" + fmtTime(row.created_at) + "</td>"
      + "<td><span class=\"source-site\">" + sourceBadge(row.source_type) + "<a class=\"site-link\" href=\"" + escapeAttr(loginURL(row.site_base_url)) + "\" target=\"_blank\" rel=\"noopener noreferrer\">" + escapeHTML(row.site_name || "") + "</a></span></td>"
      + "<td>" + categoryTag(row.category, row.category_name) + "</td>"
      + "<td>" + escapeHTML(row.model_keyword || "") + "</td>"
      + "<td>" + escapeHTML(row.model_name || "") + "</td>"
      + "<td><span class=\"group-badge\">" + escapeHTML(row.group_name || "") + "</span>" + invalidBadge + invalidReason + "</td>"
      + "<td>" + fmt(row.group_ratio) + "</td>"
      + "<td>" + fmtBalance(row.upstream_balance, row.balance_unit) + "</td>"
      + "<td>" + fmt(effectivePrice(row)) + "</td>"
      + "<td>" + fmt(row.input_price) + "</td>"
      + "<td>" + fmt(row.output_price) + "</td>"
      + "<td>" + fmt(row.cache_read_price) + "</td>"
      + "<td>" + fmt(row.cache_write_price) + "</td>"
      + "<td>" + fmt(row.request_price) + "</td>"
      + "</tr>";
  }).join("") : "<tr><td class=\"empty-state\" colspan=\"14\">暂无价格快照，运行一次监控规则后会在这里展示最新结果。</td></tr>";
}

function renderSites() {
  renderSiteListTabs();
  const body = $("#sitesList");
  if (!body) return;
  const activeType = state.activeSiteListType === "sub2api" ? "sub2api" : "newapi";
  const rows = state.sites.filter((site) => {
    const sourceType = String(site.source_type || "newapi").toLowerCase() === "sub2api" ? "sub2api" : "newapi";
    return sourceType === activeType;
  });
  body.innerHTML = rows.length ? rows.map((site) => {
    const error = site.last_error ? "<div class=\"error\">" + escapeHTML(site.last_error) + "</div>" : "";
    const sourceType = String(site.source_type || "newapi").toLowerCase() === "sub2api" ? "sub2api" : "newapi";
    const editAttr = sourceType === "sub2api" ? "data-edit-sub2-upstream" : "data-edit-site";
    const deleteAttr = sourceType === "sub2api" ? "data-delete-sub2-upstream" : "data-delete-site";
    return "<tr>"
      + "<td>" + site.id + "</td>"
      + "<td>" + sourceBadge(sourceType) + " <a class=\"site-link\" href=\"" + escapeAttr(loginURL(site.base_url)) + "\" target=\"_blank\" rel=\"noopener noreferrer\">" + escapeHTML(site.name) + "</a></td>"
      + "<td>" + escapeHTML(site.base_url || "") + "</td>"
      + "<td>" + escapeHTML(site.username || "") + "</td>"
      + "<td>" + (fmtTime(site.last_run_at) || "尚未验证/采集") + error + "</td>"
      + "<td><div class=\"table-actions\">"
      + "<button class=\"secondary\" " + editAttr + "=\"" + site.id + "\" type=\"button\">编辑</button>"
      + "<button class=\"danger\" " + deleteAttr + "=\"" + site.id + "\" type=\"button\">删除</button>"
      + "</div></td>"
      + "</tr>";
  }).join("") : "<tr><td class=\"empty-state\" colspan=\"6\">暂无" + (activeType === "sub2api" ? " sub2api " : " NewAPI ") + "上游站点。</td></tr>";
}

function renderSiteSourceTabs() {
  document.querySelectorAll("[data-site-source-tab]").forEach((button) => {
    const active = button.getAttribute("data-site-source-tab") === state.activeSiteSourceType;
    button.classList.toggle("active", active);
    button.setAttribute("aria-selected", active ? "true" : "false");
  });
}

function renderSiteListTabs() {
  document.querySelectorAll("[data-site-list-tab]").forEach((button) => {
    const active = button.getAttribute("data-site-list-tab") === state.activeSiteListType;
    button.classList.toggle("active", active);
    button.setAttribute("aria-selected", active ? "true" : "false");
  });
}

function renderSettings() {
  const form = $("#settingsForm");
  if (!form || !state.settings) return;
  form.elements.sub2api_enabled.checked = !!state.settings.sub2api_enabled;
  const syncStatus = $("#sub2apiSyncStatus");
  if (syncStatus) {
    const enabled = !!state.settings.sub2api_enabled;
    syncStatus.textContent = enabled ? "已开启" : "未开启";
    syncStatus.className = "sync-status " + (enabled ? "on" : "off");
  }
  form.elements.sub2api_main_base_url.value = state.settings.sub2api_main_base_url || state.settings.sub2api_base_url || "";
  const savedKey = state.settings.sub2api_admin_key || state.settings.sub2api_access_token || "";
  form.elements.sub2api_admin_key.placeholder = savedKey ? "已保存：" + savedKey : "主站后台生成的 admin-...，留空保留现有值";
  form.elements.sub2api_admin_key.value = "";
  form.elements.sync_threshold_ratio.value = state.settings.sync_threshold_ratio ? String(state.settings.sync_threshold_ratio) : "";
  form.elements.email_notify_enabled.checked = !!state.settings.email_notify_enabled;
  form.elements.email_notify_price_change.checked = state.settings.email_notify_price_change !== false;
  form.elements.email_notify_sync_update.checked = state.settings.email_notify_sync_update !== false;
  form.elements.smtp_host.value = state.settings.smtp_host || "";
  form.elements.smtp_port.value = String(state.settings.smtp_port || 587);
  form.elements.smtp_encryption.value = state.settings.smtp_encryption || "auto";
  form.elements.smtp_username.value = state.settings.smtp_username || "";
  const savedSMTPPassword = state.settings.smtp_password || "";
  form.elements.smtp_password.placeholder = savedSMTPPassword ? "已保存：" + savedSMTPPassword : "留空表示保留现有密码";
  form.elements.smtp_password.value = "";
  form.elements.smtp_from.value = state.settings.smtp_from || "";
  form.elements.smtp_to.value = state.settings.smtp_to || "";
}

function renderSub2Accounts() {
  const body = $("#sub2AccountsBody");
  if (!body) return;
  body.innerHTML = state.sub2Accounts.length ? state.sub2Accounts.map((account) => {
    const baseURL = account.credentials?.base_url || "";
    const groups = account.groups?.length
      ? account.groups.map((group) => group.name + " #" + group.id).join(", ")
      : (account.group_ids || []).map((id) => "#" + id).join(", ");
    const enabled = account.status === "active" && account.schedulable;
    return "<tr>"
      + "<td>" + account.id + "</td>"
      + "<td>" + escapeHTML(account.name || "") + "<div class=\"muted\">" + escapeHTML(account.platform || "") + " / " + escapeHTML(account.type || "") + "</div></td>"
      + "<td>" + escapeHTML(baseURL) + "</td>"
      + "<td>" + escapeHTML(groups || "") + "</td>"
      + "<td><span class=\"" + (enabled ? "status" : "status disabled") + "\">" + (enabled ? "启用" : "停用") + "</span></td>"
      + "<td><div class=\"table-actions\">"
      + "<button class=\"secondary\" data-sub2-enable=\"" + account.id + "\" type=\"button\">启用</button>"
      + "<button class=\"danger\" data-sub2-disable=\"" + account.id + "\" type=\"button\">停用</button>"
      + "<button class=\"secondary\" data-sub2-update-key=\"" + account.id + "\" type=\"button\">更新 key</button>"
      + "</div></td>"
      + "</tr>";
  }).join("") : "<tr><td class=\"empty-state\" colspan=\"6\">未加载主站渠道账号。可填写 apiurl 后搜索。</td></tr>";
}

function renderSub2UserPrices() {
  const body = $("#sub2UserPricesBody");
  const summary = $("#sub2UserPriceSummary");
  const controls = $("#sub2UserPriceControls");
  const pager = $("#sub2UserPricePager");
  const searchInput = $("#sub2UserPriceSearch");
  const pageSizeSelect = $("#sub2UserPricePageSize");
  if (!body) return;
  const result = state.sub2UserPrices;
  if (!result) {
    body.innerHTML = "<tr><td class=\"empty-state\" colspan=\"8\">尚未获取用户分组倍率和模型价格。</td></tr>";
    if (summary) summary.textContent = "尚未获取用户价格。";
    if (controls) controls.hidden = true;
    if (pager) pager.hidden = true;
    return;
  }
  if (controls) controls.hidden = false;
  if (searchInput && searchInput.value !== state.sub2UserPriceSearch) searchInput.value = state.sub2UserPriceSearch;
  if (pageSizeSelect && String(pageSizeSelect.value) !== String(state.sub2UserPricePageSize)) pageSizeSelect.value = String(state.sub2UserPricePageSize);
  const rows = filteredSub2UserPriceRows(result.rows || []);
  const totalPages = Math.max(1, Math.ceil(rows.length / state.sub2UserPricePageSize));
  if (state.sub2UserPricePage > totalPages) state.sub2UserPricePage = totalPages;
  if (state.sub2UserPricePage < 1) state.sub2UserPricePage = 1;
  const start = (state.sub2UserPricePage - 1) * state.sub2UserPricePageSize;
  const pageRows = rows.slice(start, start + state.sub2UserPricePageSize);
  if (summary) {
    const cheapestText = (result.cheapest_groups || []).map((group) => (group.platform_label || group.platform) + ":" + group.group_name + "(" + fmt(group.effective_rate) + ")").join("，");
    const allRows = result.rows || [];
    const range = rows.length ? "，当前 " + (start + 1) + "-" + Math.min(start + pageRows.length, rows.length) : "";
    summary.textContent = "已获取 " + (result.groups?.length || 0) + " 个可用分组，匹配 " + rows.length + " / " + (result.total_rows || allRows.length) + " 条价格" + range + "，来源：" + (result.price_source || "官方价格库") + (cheapestText ? "。最低倍率分组：" + cheapestText : "");
  }
  body.innerHTML = pageRows.length ? pageRows.map((row) => {
    const rate = row.user_group_rate === null || row.user_group_rate === undefined
      ? fmt(row.effective_rate)
      : fmt(row.effective_rate) + "（用户覆盖）";
    return "<tr>"
      + "<td>" + escapeHTML(row.model || "") + "<div class=\"muted\">" + escapeHTML(row.mode || "") + "</div></td>"
      + "<td>" + escapeHTML(row.provider || "") + "</td>"
      + "<td><span class=\"group-badge\">" + escapeHTML(row.group_name || "") + "</span><div class=\"muted\">#" + escapeHTML(row.group_id || "") + " " + escapeHTML(row.group_platform || "") + "</div></td>"
      + "<td>" + escapeHTML(rate) + "<div class=\"muted\">默认：" + fmt(row.group_default_rate) + "</div></td>"
      + "<td>" + fmt(row.final_input_per_1m_tokens) + "<div class=\"muted\">官方：" + fmt(row.official_input_per_1m_tokens) + "</div></td>"
      + "<td>" + fmt(row.final_output_per_1m_tokens) + "<div class=\"muted\">官方：" + fmt(row.official_output_per_1m_tokens) + "</div></td>"
      + "<td>" + fmt(row.final_cache_write_per_1m_tokens) + "<div class=\"muted\">官方：" + fmt(row.official_cache_write_per_1m_tokens) + "</div></td>"
      + "<td>" + fmt(row.final_cache_read_per_1m_tokens) + "<div class=\"muted\">官方：" + fmt(row.official_cache_read_per_1m_tokens) + "</div></td>"
      + "</tr>";
  }).join("") : "<tr><td class=\"empty-state\" colspan=\"8\">没有匹配的模型价格。请调整关键词、分组或 provider 过滤。</td></tr>";
  renderSub2UserPricePager(rows.length, totalPages);
}

function filteredSub2UserPriceRows(rows) {
  const keyword = String(state.sub2UserPriceSearch || "").trim().toLowerCase();
  if (!keyword) return rows;
  return rows.filter((row) => {
    return [
      row.model,
      row.provider,
      row.group_name,
      row.group_platform,
      row.mode,
    ].some((value) => String(value || "").toLowerCase().includes(keyword));
  });
}

function renderSub2UserPricePager(totalRows, totalPages) {
  const pager = $("#sub2UserPricePager");
  if (!pager) return;
  if (!state.sub2UserPrices) {
    pager.hidden = true;
    pager.innerHTML = "";
    return;
  }
  pager.hidden = false;
  pager.innerHTML = "<span class=\"pager-info\">第 " + state.sub2UserPricePage + " / " + totalPages + " 页，共 " + totalRows + " 条</span>"
    + "<button class=\"secondary\" type=\"button\" data-sub2-price-page=\"prev\"" + (state.sub2UserPricePage <= 1 ? " disabled" : "") + ">上一页</button>"
    + "<button class=\"secondary\" type=\"button\" data-sub2-price-page=\"next\"" + (state.sub2UserPricePage >= totalPages ? " disabled" : "") + ">下一页</button>";
}

function renderSub2UserFilterOptions() {
  const status = $("#sub2UserFilterStatus");
  const panel = $("#sub2UserFilterOptions");
  if (!status || !panel) return;
  if (state.sub2UserFilterLoading) {
    status.textContent = "正在读取当前 sub2api 账号的可选过滤项...";
    panel.innerHTML = "";
    return;
  }
  const options = state.sub2UserFilterOptions;
  if (!options) {
    status.textContent = "选择已保存站点或填写站点登录信息后，会自动读取可选过滤项。";
    panel.innerHTML = "";
    return;
  }
  const summary = options.summary || {};
  status.textContent = "已读取 " + (summary.group_count || (options.groups || []).length || 0)
    + " 个分组、" + (summary.model_count || 0) + " 个官方模型，可点击下方选项写入过滤条件，也可以继续手动编辑。";
  const groups = (options.groups || []).map((group) => ({
    value: group.name || String(group.id),
    label: (group.name || ("#" + group.id)) + " · " + (group.platform || "unknown"),
    count: group.rate_multiplier || group.rate || "",
  }));
  panel.innerHTML = [
    filterOptionGroupHTML("平台", "platforms", options.platforms || []),
    filterOptionGroupHTML("分组", "groups", groups),
    filterOptionGroupHTML("Provider", "providers", options.providers || []),
    filterOptionGroupHTML("模式", "modes", options.modes || []),
    filterOptionGroupHTML("模型", "models", options.models || []),
  ].filter(Boolean).join("");
}

function filterOptionGroupHTML(title, target, options) {
  if (!options.length) return "";
  const buttons = options.slice(0, target === "models" ? 80 : 120).map((option) => {
    const suffix = option.count ? " (" + escapeHTML(option.count) + ")" : "";
    return "<button class=\"filter\" type=\"button\" data-dynamic-filter-target=\"" + escapeAttr(target) + "\" data-dynamic-filter-value=\"" + escapeAttr(option.value) + "\">"
      + escapeHTML(option.label || option.value) + suffix + "</button>";
  }).join("");
  return "<div class=\"filter-option-group\"><div class=\"filter-option-title\">" + escapeHTML(title) + "</div><div class=\"filter-option-buttons\">" + buttons + "</div></div>";
}

function sub2UserCredentialPayload() {
  const form = currentSiteSourceType() === "sub2api" ? $("#siteForm") : null;
  if (!form) return {};
  const payload = formJSON(form);
  if (!payload.email && payload.username) payload.email = payload.username;
  return payload;
}

function sub2ChannelPayload() {
  const form = $("#sub2ChannelForm");
  if (!form) return {};
  return formJSON(form);
}

async function searchSub2Accounts() {
  const payload = sub2ChannelPayload();
  const params = new URLSearchParams();
  if (payload.apiurl) params.set("apiurl", payload.apiurl);
  state.sub2Accounts = await api("/api/sub2api/accounts?" + params.toString()) || [];
  renderSub2Accounts();
}

function sub2UserPricePayload() {
 const form = $("#sub2UserPriceForm");
 if (!form) return {};
 const payload = formJSON(form);
  const credentials = selectedSub2UserPriceUpstream() || sub2UserCredentialPayload();
  payload.sub2api_upstream_id = Number(payload.sub2api_upstream_id || 0);
  payload.base_url = payload.base_url || "";
  payload.email = payload.email || credentials.email || "";
  payload.password = payload.password || credentials.password || "";
  payload.auth_token = payload.auth_token || credentials.auth_token || "";
  payload.totp_code = payload.totp_code || credentials.totp_code || "";
  payload.platforms = payload.platforms || credentials.platforms || "";
  payload.limit = Number(payload.limit || 500);
  return payload;
}

function sub2UserFilterOptionsPayload() {
  const payload = sub2UserPricePayload();
  payload.limit = 1;
  return payload;
}

function hasSub2UserFilterSource(payload) {
  return !!(payload.sub2api_upstream_id || (payload.base_url && (payload.auth_token || (payload.email && payload.password))));
}

function selectedSub2UserPriceUpstream() {
  const form = $("#sub2UserPriceForm");
  if (!form) return null;
  const id = Number(form.elements.sub2api_upstream_id?.value || 0);
  if (!id) return null;
  return state.sub2Upstreams.find((item) => item.id === id) || null;
}

function applySub2UserPriceSource(source) {
  const form = $("#sub2UserPriceForm");
  if (!form) return;
  const values = source || {};
  ["base_url", "email", "password", "auth_token", "totp_code"].forEach((name) => {
    const input = form.elements[name];
    if (input) input.value = values[name] || "";
  });
}

function syncSub2UserPriceUpstreamFields() {
  const form = $("#sub2UserPriceForm");
  if (!form) return;
  const upstream = selectedSub2UserPriceUpstream();
  const locked = !!upstream;
  const values = upstream || {};
  const fields = ["base_url", "email", "password", "auth_token", "totp_code"];
  fields.forEach((name) => {
    const input = form.elements[name];
    if (!input) return;
    input.readOnly = locked;
    input.setAttribute("aria-readonly", locked ? "true" : "false");
    if (locked) {
      input.value = values[name] || "";
    }
  });
  if (!locked) {
    fields.forEach((name) => {
      const input = form.elements[name];
      if (!input) return;
      input.readOnly = false;
      input.removeAttribute("aria-readonly");
    });
  }
}

function syncSub2UserPriceFromAccountForm() {
  const form = $("#sub2UserPriceForm");
  if (!form || selectedSub2UserPriceUpstream()) return;
  const credentials = sub2UserCredentialPayload();
  if (!credentials.base_url) return;
  applySub2UserPriceSource(credentials);
}

function toggleCSVValue(input, value) {
  if (!input || !value) return;
  const parts = input.value.split(",").map((item) => item.trim()).filter(Boolean);
  const exists = parts.some((item) => item.toLowerCase() === value.toLowerCase());
  input.value = exists
    ? parts.filter((item) => item.toLowerCase() !== value.toLowerCase()).join(",")
    : [...parts, value].join(",");
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

function applyQuickFilter(button) {
  const container = button.closest("[data-fill-target]");
  const form = $("#sub2UserPriceForm");
  if (!container || !form) return;
  const input = form.elements[container.getAttribute("data-fill-target") || ""];
  const value = button.getAttribute("data-fill-value") || "";
  if (input?.name === "model_keyword") {
    input.value = value;
    input.dispatchEvent(new Event("input", { bubbles: true }));
    return;
  }
  toggleCSVValue(input, value);
}

async function loadSub2UserFilterOptionsNow() {
  const payload = sub2UserFilterOptionsPayload();
  if (!hasSub2UserFilterSource(payload)) {
    state.sub2UserFilterOptions = null;
    state.sub2UserFilterLoading = false;
    renderSub2UserFilterOptions();
    return;
  }
  const requestId = ++sub2UserFilterRequestId;
  state.sub2UserFilterLoading = true;
  renderSub2UserFilterOptions();
  try {
    const result = await api("/api/sub2api/user-filter-options", { method: "POST", body: JSON.stringify(payload) });
    if (requestId !== sub2UserFilterRequestId) return;
    state.sub2UserFilterOptions = result;
  } catch (error) {
    if (requestId !== sub2UserFilterRequestId) return;
    state.sub2UserFilterOptions = null;
    toast(error.message);
  } finally {
    if (requestId === sub2UserFilterRequestId) {
      state.sub2UserFilterLoading = false;
      renderSub2UserFilterOptions();
    }
  }
}

function scheduleSub2UserFilterOptionsLoad() {
  clearTimeout(sub2UserFilterTimer);
  sub2UserFilterTimer = setTimeout(() => {
    loadSub2UserFilterOptionsNow().catch((error) => toast(error.message));
  }, 450);
}

function renderSortHeaders() {
  document.querySelectorAll("th[data-sort]").forEach((th) => {
    const key = th.getAttribute("data-sort");
    th.classList.toggle("sorted-asc", state.sort.key === key && state.sort.dir === "asc");
    th.classList.toggle("sorted-desc", state.sort.key === key && state.sort.dir === "desc");
  });
}

function sourceBadge(sourceType) {
  const isSub2API = String(sourceType || "").toLowerCase() === "sub2api";
  const label = isSub2API ? "sub2api" : "NewAPI";
  const className = isSub2API ? "sub2api" : "newapi";
  return "<span class=\"source-badge " + className + "\">" + label + "</span>";
}

function sortedSnapshots() {
  const rows = [...state.snapshots];
  rows.sort((left, right) => compareSnapshot(left, right, state.sort.key, state.sort.dir));
  return rows;
}

function compareSnapshot(left, right, key, dir) {
  if (!!left.invalid !== !!right.invalid) return left.invalid ? 1 : -1;
  const factor = dir === "desc" ? -1 : 1;
  const numericKeys = new Set(["group_ratio", "upstream_balance", "effective_price", "input_price", "output_price", "cache_read_price", "cache_write_price", "request_price"]);
  let result;
  if (key === "created_at") {
    result = new Date(left.created_at || 0).getTime() - new Date(right.created_at || 0).getTime();
  } else if (key === "effective_price") {
    result = numberSort(effectivePrice(left), effectivePrice(right));
  } else if (numericKeys.has(key)) {
    result = numberSort(left[key], right[key]);
  } else {
    result = String(left[key] || "").localeCompare(String(right[key] || ""), "zh-Hans-CN", { numeric: true, sensitivity: "base" });
  }
  if (result === 0 && key !== "model_name") {
    result = String(left.model_name || "").localeCompare(String(right.model_name || ""), "zh-Hans-CN", { numeric: true, sensitivity: "base" });
  }
  if (result === 0) {
    result = numberSort(effectivePrice(left), effectivePrice(right));
  }
  return result * factor;
}

function numberSort(left, right) {
  const a = left === null || left === undefined || left === "" ? Number.POSITIVE_INFINITY : Number(left);
  const b = right === null || right === undefined || right === "" ? Number.POSITIVE_INFINITY : Number(right);
  return a - b;
}

function effectivePrice(row) {
  return row.input_price ?? row.request_price ?? row.output_price ?? Number.POSITIVE_INFINITY;
}

function categoryBySlug(slug) {
  return state.categories.find((category) => category.slug === slug);
}

function categoryTag(slug, name) {
  const category = categoryBySlug(slug) || { slug: slug || "other", name: name || slug || "其他" };
  const className = "category-tag " + safeClass(category.slug);
  return "<span class=\"" + className + "\">" + escapeHTML(name || category.name) + "</span>";
}

function safeClass(value) {
  const normalized = String(value || "other").toLowerCase().replace(/[^a-z0-9_-]/g, "-");
  return normalized || "other";
}

function loginURL(baseURL) {
  try {
    return new URL("login", baseURL || window.location.origin).toString();
  } catch (_error) {
    return baseURL || "#";
  }
}

function currentSiteSourceType() {
  return state.activeSiteSourceType === "sub2api" ? "sub2api" : "newapi";
}

function setSiteSourceType(sourceType) {
  state.activeSiteSourceType = sourceType === "sub2api" ? "sub2api" : "newapi";
  const input = $("#siteSourceTypeInput");
  if (input) input.value = state.activeSiteSourceType;
  syncSiteSourceFields();
}

function syncSiteSourceFields() {
  const sourceType = currentSiteSourceType();
  renderSiteSourceTabs();
  document.querySelectorAll("[data-site-source-field]").forEach((field) => {
    const active = field.getAttribute("data-site-source-field") === sourceType;
    field.hidden = !active;
    field.querySelectorAll("input,select").forEach((input) => {
      input.disabled = !active;
    });
  });
  const form = $("#siteForm");
  if (form?.elements.source_type) {
    form.elements.source_type.value = sourceType;
  }
  if (form?.elements.username) {
    form.elements.username.placeholder = sourceType === "sub2api" ? "sub2api 登录邮箱或用户名" : "用户名或邮箱";
    form.elements.username.required = true;
  }
  if (form?.elements.password) {
    const editing = sourceType === "sub2api" ? !!state.editingSub2UpstreamId : !!state.editingSiteId;
    form.elements.password.required = !editing;
  }
}

function editSite(site) {
  const form = $("#siteForm");
  if (!form) return;
  setActiveView("monitor");
  state.editingSiteId = site.id;
  state.editingSub2UpstreamId = null;
  state.activeSiteSourceType = "newapi";
  state.activeSiteListType = "newapi";
  form.elements.id.value = site.id;
  form.elements.name.value = site.name || "";
  form.elements.base_url.value = site.base_url || "";
  form.elements.username.value = site.username || "";
  if (form.elements.auth_token) form.elements.auth_token.value = "";
  form.elements.password.value = "";
  form.elements.password.required = false;
  form.elements.password.placeholder = "留空表示不修改密码";
  form.elements.totp_code.value = "";
  syncSiteSourceFields();
  setText("#siteFormTitle", "编辑 NewAPI 上游");
  setText("#siteFormHelp", "修改 NewAPI 上游信息；密码留空时会保留原密码。");
  setText("#siteSubmitBtn", "更新站点");
  const cancel = $("#siteCancelBtn");
  if (cancel) cancel.hidden = false;
  form.scrollIntoView({ behavior: "smooth", block: "start" });
}

function editSub2Site(upstream) {
  const form = $("#siteForm");
  if (!form) return;
  setActiveView("monitor");
  state.editingSiteId = null;
  state.editingSub2UpstreamId = upstream.id;
  state.activeSiteSourceType = "sub2api";
  state.activeSiteListType = "sub2api";
  form.elements.id.value = upstream.id;
  form.elements.name.value = upstream.name || "";
  form.elements.base_url.value = upstream.base_url || "";
  form.elements.username.value = upstream.email || upstream.username || "";
  form.elements.password.value = "";
  form.elements.password.placeholder = "留空表示不修改密码";
  form.elements.auth_token.value = "";
  form.elements.auth_token.placeholder = upstream.auth_token ? "已保存：" + upstream.auth_token : "可选，填写后不使用密码登录";
  form.elements.totp_code.value = "";
  syncSiteSourceFields();
  setText("#siteFormTitle", "编辑 sub2api 上游");
  setText("#siteFormHelp", "修改 sub2api 上游账号；密码和 Auth Token 留空时保留原值。");
  setText("#siteSubmitBtn", "更新上游");
  const cancel = $("#siteCancelBtn");
  if (cancel) cancel.hidden = false;
  form.scrollIntoView({ behavior: "smooth", block: "start" });
}

function resetSiteForm() {
  const form = $("#siteForm");
  if (!form) return;
  state.editingSiteId = null;
  state.editingSub2UpstreamId = null;
  form.reset();
  form.elements.id.value = "";
  state.activeSiteSourceType = "newapi";
  form.elements.source_type.value = "newapi";
  form.elements.password.required = true;
  form.elements.password.placeholder = "";
  if (form.elements.auth_token) form.elements.auth_token.placeholder = "可选，填写后不使用密码登录";
  syncSiteSourceFields();
  setText("#siteFormTitle", "添加上游站点账号");
  setText("#siteFormHelp", "用上方切换选择 NewAPI 或 sub2api，账号保存后会进入对应列表。");
  setText("#siteSubmitBtn", "保存上游");
  const cancel = $("#siteCancelBtn");
  if (cancel) cancel.hidden = true;
}

async function editCategory(category) {
  const form = $("#categoryForm");
  if (!form) return;
  setActiveView("settings");
  const groups = categoryMainGroups(category);
  state.editingCategoryId = category.id;
  form.elements.id.value = category.id;
  form.elements.name.value = category.name || "";
  form.elements.slug.value = category.slug || "";
  form.elements.sub2api_main_group_name.value = category.sub2api_main_group_name || "";
  form.elements.sub2api_main_group_id.value = String(category.sub2api_main_group_id || 0);
  form.dataset.mainGroupIds = groups.map((group) => String(group.id || "")).filter(Boolean).join(",");
  form.dataset.mainGroupsJson = JSON.stringify(groups);
  renderCategoryMainGroupOptions();
  await refreshSub2MainGroups();
  setText("#categorySubmitBtn", "更新分类");
  const cancel = $("#categoryCancelBtn");
  if (cancel) cancel.hidden = false;
}

function resetCategoryForm() {
  const form = $("#categoryForm");
  if (!form) return;
  state.editingCategoryId = null;
  form.reset();
  form.elements.id.value = "";
  form.elements.sub2api_main_group_id.value = "0";
  form.dataset.mainGroupIds = "";
  form.dataset.mainGroupsJson = "";
  renderCategoryMainGroupOptions();
  setText("#categorySubmitBtn", "保存分类");
  const cancel = $("#categoryCancelBtn");
  if (cancel) cancel.hidden = true;
}

function editRule(rule) {
  const form = $("#ruleForm");
  if (!form) return;
  setActiveView("monitor");
  state.editingRuleId = rule.id;
  form.elements.id.value = rule.id;
  state.activeRuleSourceType = rule.source_type === "sub2api" ? "sub2api" : "newapi";
  form.elements.source_type.value = state.activeRuleSourceType;
  form.elements.site_id.value = String(rule.site_id || "");
  form.elements.sub2api_upstream_id.value = String(rule.sub2api_upstream_id || 0);
  form.elements.category.value = rule.category || "other";
  form.elements.model_keyword.value = rule.model_keyword || rule.model_name || "";
  form.elements.sync_base_group.value = rule.sync_base_group || rule.group_name || "";
  form.elements.interval_minutes.value = String(rule.interval_minutes || 15);
  form.elements.schedule_enabled.checked = !!rule.schedule_enabled;
  form.elements.sync_enabled.checked = !!rule.sync_enabled;
  syncRuleSourceFields();
  setText("#ruleFormTitle", "编辑监控规则");
  setText("#ruleFormHelp", "修改后会影响后续监控采集；历史快照保留原记录。");
  setText("#ruleSubmitBtn", "更新规则");
  const cancel = $("#ruleCancelBtn");
  if (cancel) cancel.hidden = false;
  form.scrollIntoView({ behavior: "smooth", block: "start" });
}

function resetRuleForm() {
  const form = $("#ruleForm");
  if (!form) return;
  state.editingRuleId = null;
  form.reset();
  form.elements.id.value = "";
  state.activeRuleSourceType = "newapi";
  form.elements.source_type.value = "newapi";
  form.elements.sub2api_upstream_id.value = "0";
  form.elements.interval_minutes.value = "15";
  form.elements.schedule_enabled.checked = true;
  form.elements.sync_enabled.checked = false;
  syncRuleSourceFields();
  setText("#ruleFormTitle", "添加监控规则");
  setText("#ruleFormHelp", "选择 NewAPI 或 sub2api 上游渠道，按完整模型名监控指定模型，按真实价格优先排序并写入价格快照。");
  setText("#ruleSubmitBtn", "保存规则");
  const cancel = $("#ruleCancelBtn");
  if (cancel) cancel.hidden = true;
}

function escapeHTML(value) {
  return String(value ?? "").replace(/[&<>"']/g, (char) => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    "\"": "&quot;",
    "'": "&#39;",
  }[char]));
}

function escapeAttr(value) {
  return escapeHTML(value);
}

const siteForm = $("#siteForm");
if (siteForm) {
  syncSiteSourceFields();
  siteForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    const payload = formJSON(form);
    const sourceType = currentSiteSourceType();
    const id = Number(payload.id || (sourceType === "sub2api" ? state.editingSub2UpstreamId : state.editingSiteId) || 0);
    delete payload.id;
    delete payload.source_type;
    try {
      if (sourceType === "sub2api") {
        const sub2Payload = {
          name: payload.name,
          base_url: payload.base_url,
          email: payload.username,
          password: payload.password || "",
          auth_token: payload.auth_token || "",
          totp_code: payload.totp_code || "",
        };
        if (!id && !sub2Payload.auth_token && (!sub2Payload.email || !sub2Payload.password)) {
          toast("请填写 sub2api 用户名和密码，或填写 Auth Token");
          return;
        }
        if (id) {
          await api("/api/sub2api/upstreams/" + id + "/update", { method: "POST", body: JSON.stringify(sub2Payload) });
          toast("sub2api 上游已更新");
        } else {
          await api("/api/sub2api/upstreams", { method: "POST", body: JSON.stringify(sub2Payload) });
          toast("sub2api 上游已保存");
        }
      } else {
        delete payload.auth_token;
        if (!payload.password) delete payload.password;
        if (id) {
          await api("/api/sites/" + id + "/update", { method: "POST", body: JSON.stringify(payload) });
          toast("NewAPI 上游已更新");
        } else {
          await api("/api/sites", { method: "POST", body: JSON.stringify(payload) });
          toast("NewAPI 上游已保存");
        }
      }
      resetSiteForm();
      await loadAll();
    } catch (error) {
      toast(error.message);
    }
  });
}

const loginForm = $("#loginForm");
if (loginForm) {
  loginForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    const button = form.querySelector("button[type=submit]");
    if (button) {
      button.disabled = true;
      button.textContent = "登录中";
    }
    setLoginError("");
    try {
      await api("/api/auth/login", { method: "POST", body: JSON.stringify(formJSON(form)) });
      form.reset();
      hideLogin();
    } catch (error) {
      setLoginError(error.message);
      toast(error.message);
      return;
    } finally {
      if (button) {
        button.disabled = false;
        button.textContent = "登录";
      }
    }
    try {
      await loadAll();
    } catch (error) {
      toast("登录成功，但加载数据失败：" + error.message);
    }
  });
}

const settingsForm = $("#settingsForm");
if (settingsForm) {
  settingsForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    const payload = formJSON(form);
    payload.smtp_port = Number(payload.smtp_port || 587);
    payload.sync_threshold_ratio = Number(payload.sync_threshold_ratio || 0);
    try {
      await api("/api/settings", { method: "POST", body: JSON.stringify(payload) });
      toast("主站 sub2api 设置已保存");
      await loadAll();
    } catch (error) {
      toast(error.message);
    }
  });
}

const adminPasswordForm = $("#adminPasswordForm");
if (adminPasswordForm) {
  adminPasswordForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    const button = form.querySelector("button[type=submit]");
    if (button) {
      button.disabled = true;
      button.textContent = "修改中";
    }
    try {
      await api("/api/auth/password", { method: "POST", body: JSON.stringify(formJSON(form)) });
      form.reset();
      toast("登录密码已修改，请重新登录");
      showLogin();
    } catch (error) {
      toast(error.message);
    } finally {
      if (button) {
        button.disabled = false;
        button.textContent = "修改登录密码";
      }
    }
  });
}

const sub2SearchBtn = $("#sub2SearchBtn");
if (sub2SearchBtn) {
  sub2SearchBtn.addEventListener("click", async () => {
    sub2SearchBtn.disabled = true;
    try {
      await searchSub2Accounts();
      toast("sub2api 渠道账号已加载");
    } catch (error) {
      toast(error.message);
    } finally {
      sub2SearchBtn.disabled = false;
    }
  });
}

const sub2UserPriceBtn = $("#sub2UserPriceBtn");
if (sub2UserPriceBtn) {
  sub2UserPriceBtn.addEventListener("click", async () => {
    const payload = sub2UserPricePayload();
    if (!payload.sub2api_upstream_id && !payload.auth_token && (!payload.email || !payload.password)) {
      toast("请填写 sub2api 用户名和密码，或填写 Auth Token");
      return;
    }
    sub2UserPriceBtn.disabled = true;
    sub2UserPriceBtn.textContent = "获取中";
    try {
      state.sub2UserPrices = await api("/api/sub2api/user-prices", { method: "POST", body: JSON.stringify(payload) });
      state.sub2UserPricePage = 1;
      renderSub2UserPrices();
      toast("sub2api 用户分组倍率和模型价格已获取");
    } catch (error) {
      toast(error.message);
    } finally {
      sub2UserPriceBtn.disabled = false;
      sub2UserPriceBtn.textContent = "获取分组倍率和模型价格";
    }
  });
}

const sub2UserPriceSearch = $("#sub2UserPriceSearch");
if (sub2UserPriceSearch) {
  sub2UserPriceSearch.addEventListener("input", (event) => {
    state.sub2UserPriceSearch = event.currentTarget.value || "";
    state.sub2UserPricePage = 1;
    renderSub2UserPrices();
  });
}

const sub2UserPricePageSize = $("#sub2UserPricePageSize");
if (sub2UserPricePageSize) {
  sub2UserPricePageSize.addEventListener("change", (event) => {
    state.sub2UserPricePageSize = Number(event.currentTarget.value || 25);
    state.sub2UserPricePage = 1;
    renderSub2UserPrices();
  });
}

const sub2UserPriceUpstreamSelect = $("#sub2UserPriceUpstreamSelect");
if (sub2UserPriceUpstreamSelect) {
  sub2UserPriceUpstreamSelect.addEventListener("change", () => {
    syncSub2UserPriceUpstreamFields();
    loadSub2UserFilterOptionsNow().catch((error) => toast(error.message));
  });
}

const sub2UserPriceForm = $("#sub2UserPriceForm");
if (sub2UserPriceForm) {
  ["base_url", "email", "password", "auth_token", "totp_code", "price_url", "model_keyword"].forEach((name) => {
    const input = sub2UserPriceForm.elements[name];
    if (!input) return;
    input.addEventListener("input", () => {
      if (selectedSub2UserPriceUpstream() && ["base_url", "email", "password", "auth_token", "totp_code"].includes(name)) return;
      scheduleSub2UserFilterOptionsLoad();
    });
  });
}

const categoryForm = $("#categoryForm");
if (categoryForm) {
  const mainGroupSelect = $("#categoryMainGroupSelect");
  if (mainGroupSelect) {
    mainGroupSelect.addEventListener("change", applyCategoryMainGroupSelection);
  }
  ["sub2api_main_group_name", "sub2api_main_group_id"].forEach((name) => {
    const input = categoryForm.elements[name];
    if (!input) return;
    input.addEventListener("input", () => {
      const select = $("#categoryMainGroupSelect");
      if (select) {
        Array.from(select.options).forEach((option) => { option.selected = false; });
      }
      categoryForm.dataset.mainGroupIds = "";
      categoryForm.dataset.mainGroupsJson = "";
      syncCategoryMainGroupChoiceState();
      updateCategoryMainGroupSummary();
    });
  });
  categoryForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    const payload = formJSON(form);
    const id = Number(payload.id || state.editingCategoryId || 0);
    delete payload.id;
    payload.sub2api_main_groups = selectedCategoryMainGroups();
    payload.sub2api_main_group_id = Number(payload.sub2api_main_group_id || 0);
    payload.sub2api_main_group_name = payload.sub2api_main_group_name || "";
    try {
      if (id) {
        await api("/api/categories/" + id + "/update", { method: "POST", body: JSON.stringify(payload) });
        toast("分类已更新");
      } else {
        await api("/api/categories", { method: "POST", body: JSON.stringify(payload) });
        toast("分类已保存");
      }
      resetCategoryForm();
      await loadAll();
    } catch (error) {
      toast(error.message);
    }
  });
}

const ruleForm = $("#ruleForm");
if (ruleForm) {
  syncRuleSourceFields();
  ruleForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    const payload = formJSON(form);
    const id = Number(payload.id || state.editingRuleId || 0);
    delete payload.id;
    payload.source_type = currentRuleSourceType();
    payload.interval_minutes = Number(payload.interval_minutes || 15);
    payload.enabled = true;
    if (payload.source_type === "sub2api") {
      payload.site_id = 0;
      payload.sub2api_upstream_id = Number(payload.sub2api_upstream_id || 0);
      if (!payload.sub2api_upstream_id) {
        toast(state.sub2Upstreams.length ? "请选择 sub2api 上游站点" : "请先在监控台添加 sub2api 上游站点");
        return;
      }
    } else {
      payload.site_id = Number(payload.site_id || 0);
      payload.sub2api_upstream_id = 0;
      if (!payload.site_id) {
        toast(newapiSites().length ? "请选择 NewAPI 上游站点" : "请先添加 NewAPI 上游站点");
        return;
      }
    }
    try {
      if (id) {
        await api("/api/rules/" + id + "/update", { method: "POST", body: JSON.stringify(payload) });
        toast("规则已更新");
      } else {
        await api("/api/rules", { method: "POST", body: JSON.stringify(payload) });
        toast("规则已保存");
      }
      resetRuleForm();
      await loadAll();
    } catch (error) {
      toast(error.message);
    }
  });
}

const siteCancelBtn = $("#siteCancelBtn");
if (siteCancelBtn) siteCancelBtn.addEventListener("click", resetSiteForm);

const categoryCancelBtn = $("#categoryCancelBtn");
if (categoryCancelBtn) categoryCancelBtn.addEventListener("click", resetCategoryForm);

const ruleCancelBtn = $("#ruleCancelBtn");
if (ruleCancelBtn) ruleCancelBtn.addEventListener("click", resetRuleForm);

const refreshBtn = $("#refreshBtn");
if (refreshBtn) refreshBtn.addEventListener("click", () => loadAll().catch((error) => toast(error.message)));

const logoutBtn = $("#logoutBtn");
if (logoutBtn) {
  logoutBtn.addEventListener("click", async () => {
    await api("/api/auth/logout", { method: "POST" }).catch(() => null);
    showLogin();
  });
}

document.addEventListener("click", async (event) => {
  const viewButton = event.target.closest("[data-app-view]");
  if (viewButton) {
    setActiveView(viewButton.getAttribute("data-app-view"));
    return;
  }

  const siteSourceTab = event.target.closest("[data-site-source-tab]");
  if (siteSourceTab) {
    const nextType = siteSourceTab.getAttribute("data-site-source-tab") || "newapi";
    if ((state.editingSiteId || state.editingSub2UpstreamId) && nextType !== currentSiteSourceType()) {
      resetSiteForm();
    }
    setSiteSourceType(nextType);
    return;
  }

  const siteListTab = event.target.closest("[data-site-list-tab]");
  if (siteListTab) {
    state.activeSiteListType = siteListTab.getAttribute("data-site-list-tab") === "sub2api" ? "sub2api" : "newapi";
    renderSites();
    return;
  }

  const ruleSourceTab = event.target.closest("[data-rule-source-tab]");
  if (ruleSourceTab) {
    setRuleSourceType(ruleSourceTab.getAttribute("data-rule-source-tab") || "newapi");
    return;
  }

  const filter = event.target.closest("[data-category-filter]");
  if (filter) {
    state.categoryFilter = filter.getAttribute("data-category-filter") || "all";
    try {
      await loadAll();
    } catch (error) {
      toast(error.message);
    }
    return;
  }

  const dynamicFilter = event.target.closest("[data-dynamic-filter-target]");
  if (dynamicFilter) {
    const form = $("#sub2UserPriceForm");
    const input = form?.elements[dynamicFilter.getAttribute("data-dynamic-filter-target") || ""];
    const value = dynamicFilter.getAttribute("data-dynamic-filter-value") || "";
    toggleCSVValue(input, value);
    return;
  }

  const categoryMainGroup = event.target.closest("[data-category-main-group]");
  if (categoryMainGroup) {
    toggleCategoryMainGroup(categoryMainGroup.getAttribute("data-category-main-group"));
    return;
  }

  const sub2PricePageButton = event.target.closest("[data-sub2-price-page]");
  if (sub2PricePageButton) {
    const direction = sub2PricePageButton.getAttribute("data-sub2-price-page");
    state.sub2UserPricePage += direction === "next" ? 1 : -1;
    renderSub2UserPrices();
    return;
  }

  const quickFilter = event.target.closest("[data-fill-target] button[data-fill-value]");
  if (quickFilter) {
    applyQuickFilter(quickFilter);
    return;
  }

  const sorter = event.target.closest("th[data-sort]");
  if (sorter) {
    const key = sorter.getAttribute("data-sort");
    if (state.sort.key === key) {
      state.sort.dir = state.sort.dir === "asc" ? "desc" : "asc";
    } else {
      state.sort = { key, dir: key === "created_at" ? "desc" : "asc" };
    }
    renderSnapshots();
    renderSortHeaders();
    return;
  }

  const runButton = event.target.closest("[data-run]");
  if (runButton) {
    const id = runButton.getAttribute("data-run");
    runButton.disabled = true;
    runButton.textContent = "运行中";
    try {
      const result = await api("/api/rules/" + id + "/run", { method: "POST" });
      toast("已写入 " + (result?.count || 0) + " 条价格快照");
      await loadAll();
    } catch (error) {
      toast(error.message);
    } finally {
      runButton.disabled = false;
      runButton.textContent = "运行一次";
    }
    return;
  }

  const sub2EnableButton = event.target.closest("[data-sub2-enable]");
  if (sub2EnableButton) {
    const id = Number(sub2EnableButton.getAttribute("data-sub2-enable"));
    sub2EnableButton.disabled = true;
    try {
      await api("/api/sub2api/accounts/" + id + "/enable", { method: "POST" });
      toast("sub2api 账号已启用");
      await searchSub2Accounts();
    } catch (error) {
      toast(error.message);
    } finally {
      sub2EnableButton.disabled = false;
    }
    return;
  }

  const sub2DisableButton = event.target.closest("[data-sub2-disable]");
  if (sub2DisableButton) {
    const id = Number(sub2DisableButton.getAttribute("data-sub2-disable"));
    if (!confirm("确认停用 sub2api 账号 #" + id + "？")) return;
    sub2DisableButton.disabled = true;
    try {
      await api("/api/sub2api/accounts/" + id + "/disable", { method: "POST" });
      toast("sub2api 账号已停用");
      await searchSub2Accounts();
    } catch (error) {
      toast(error.message);
    } finally {
      sub2DisableButton.disabled = false;
    }
    return;
  }

  const sub2UpdateKeyButton = event.target.closest("[data-sub2-update-key]");
  if (sub2UpdateKeyButton) {
    const id = Number(sub2UpdateKeyButton.getAttribute("data-sub2-update-key"));
    const payload = sub2ChannelPayload();
    if (!payload.api_key) {
      toast("请先在主站渠道账号列表表单中填写新 API Key");
      return;
    }
    sub2UpdateKeyButton.disabled = true;
    try {
      await api("/api/sub2api/accounts/" + id + "/apikey", { method: "POST", body: JSON.stringify(payload) });
      toast("sub2api 账号 key 已更新");
      await searchSub2Accounts();
    } catch (error) {
      toast(error.message);
    } finally {
      sub2UpdateKeyButton.disabled = false;
    }
    return;
  }

  const editSub2UpstreamButton = event.target.closest("[data-edit-sub2-upstream]");
  if (editSub2UpstreamButton) {
    const id = Number(editSub2UpstreamButton.getAttribute("data-edit-sub2-upstream"));
    const upstream = state.sub2Upstreams.find((item) => item.id === id) || sub2Sites().find((item) => item.id === id);
    if (upstream) editSub2Site(upstream);
    return;
  }

  const deleteSub2UpstreamButton = event.target.closest("[data-delete-sub2-upstream]");
  if (deleteSub2UpstreamButton) {
    const id = Number(deleteSub2UpstreamButton.getAttribute("data-delete-sub2-upstream"));
    const upstream = state.sub2Upstreams.find((item) => item.id === id) || sub2Sites().find((item) => item.id === id);
    if (!upstream || !confirm("确认删除上游 sub2api 站点账号 " + upstream.name + "？")) return;
    deleteSub2UpstreamButton.disabled = true;
    try {
      await api("/api/sub2api/upstreams/" + id + "/delete", { method: "POST" });
      if (state.editingSub2UpstreamId === id || Number($("#siteForm")?.elements.id?.value || 0) === id) resetSiteForm();
      toast("上游 sub2api 站点账号已删除");
      await loadAll();
    } catch (error) {
      toast(error.message);
    } finally {
      deleteSub2UpstreamButton.disabled = false;
    }
    return;
  }

  const editRuleButton = event.target.closest("[data-edit-rule]");
  if (editRuleButton) {
    const id = Number(editRuleButton.getAttribute("data-edit-rule"));
    const rule = state.rules.find((item) => item.id === id);
    if (rule) editRule(rule);
    return;
  }

  const deleteRuleButton = event.target.closest("[data-delete-rule]");
  if (deleteRuleButton) {
    const id = Number(deleteRuleButton.getAttribute("data-delete-rule"));
    const rule = state.rules.find((item) => item.id === id);
    const label = rule ? (rule.site_name + " / " + (rule.model_keyword || rule.model_name || "")) : ("#" + id);
    if (!confirm("删除规则会同时删除它的历史快照，确认删除 " + label + "？")) return;
    deleteRuleButton.disabled = true;
    try {
      await api("/api/rules/" + id + "/delete", { method: "POST" });
      if (state.editingRuleId === id) resetRuleForm();
      toast("规则已删除");
      await loadAll();
    } catch (error) {
      toast(error.message);
    } finally {
      deleteRuleButton.disabled = false;
    }
    return;
  }

  const editSiteButton = event.target.closest("[data-edit-site]");
  if (editSiteButton) {
    const id = Number(editSiteButton.getAttribute("data-edit-site"));
    const site = state.sites.find((item) => item.id === id);
    if (site) editSite(site);
    return;
  }

  const deleteSiteButton = event.target.closest("[data-delete-site]");
  if (deleteSiteButton) {
    const id = Number(deleteSiteButton.getAttribute("data-delete-site"));
    const site = state.sites.find((item) => item.id === id);
    if (!site || !confirm("删除站点会同时删除它的监控规则和历史快照，确认删除 " + site.name + "？")) return;
    deleteSiteButton.disabled = true;
    try {
      await api("/api/sites/" + id + "/delete", { method: "POST" });
      if (state.editingSiteId === id) resetSiteForm();
      toast("站点已删除");
      await loadAll();
    } catch (error) {
      toast(error.message);
    } finally {
      deleteSiteButton.disabled = false;
    }
    return;
  }

  const editCategoryButton = event.target.closest("[data-edit-category]");
  if (editCategoryButton) {
    const id = Number(editCategoryButton.getAttribute("data-edit-category"));
    const category = state.categories.find((item) => item.id === id);
    if (category) await editCategory(category);
    return;
  }

  const deleteCategoryButton = event.target.closest("[data-delete-category]");
  if (deleteCategoryButton) {
    const id = Number(deleteCategoryButton.getAttribute("data-delete-category"));
    const category = state.categories.find((item) => item.id === id);
    if (!category || !confirm("确认删除分类 " + category.name + "？")) return;
    deleteCategoryButton.disabled = true;
    try {
      await api("/api/categories/" + id + "/delete", { method: "POST" });
      if (state.editingCategoryId === id) resetCategoryForm();
      if (state.categoryFilter === category.slug) state.categoryFilter = "all";
      toast("分类已删除");
      await loadAll();
    } catch (error) {
      toast(error.message);
    } finally {
      deleteCategoryButton.disabled = false;
    }
  }
});

boot().catch((error) => toast(error.message));`
