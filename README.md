# NewAPI 价格监控

一个用于监控 NewAPI 与 sub2api 上游模型价格的 Go Web 项目。系统会按规则抓取模型广场/分组价格快照，做全局低价比较，并在满足同步条件时把低价上游账号同步到主站 sub2api。

## 功能

- 后台登录管理界面。
- NewAPI 上游站点和 sub2api 上游账号管理。
- 按分类、模型关键词、站点类型创建监控规则。
- 支持规则级定时监控间隔。
- 价格快照按模型精确匹配，保留同上游同分组最新数据。
- 支持无效快照标记，失效数据不参与排名。
- 规则执行时自动检测 NewAPI 今日签到状态，未签到会自动签到并在规则列表显示奖励；sub2api 上游不支持签到时显示“不支持签到功能”。
- 全局低价阈值：用官方模型价格乘阈值后，与快照中的输入、输出、缓存、请求价格综合比较。
- 主站 sub2api 同步：自动创建或更新主站账号、维护低价分组 API Key。
- 邮件通知：支持价格变动、同步成功/失败、主站账号更新、低余额跳过等事件，并可按通知类型分别配置邮件模板。
- Docker Compose 一键部署，PostgreSQL 持久化存储。

## 界面预览

### 监控台概览

![监控台概览](docs/screenshots/price-monitor-community-overview.png)

### 价格快照列表

![价格快照列表](docs/screenshots/price-monitor-community-snapshots.png)

## 快速启动

```bash
cp .env.example .env
docker compose up -d --build
```

默认访问地址：

```text
http://127.0.0.1:28080
```

首次部署前请修改 `.env` 中的后台账号、密码和 `SESSION_SECRET`。

## 本地开发

需要 Go 1.22 和 PostgreSQL。

```bash
go test ./...
go run ./cmd/server
```

常用环境变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `ADDR` | `:8080` | Web 服务监听地址 |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/newapi_price_monitor?sslmode=disable` | PostgreSQL 连接串 |
| `BASIC_AUTH_USER` | 空 | 初始后台账号 |
| `BASIC_AUTH_PASS` | 空 | 初始后台密码 |
| `SESSION_SECRET` | 自动回退 | 登录 session 密钥 |
| `MONITOR_INTERVAL` | `1m` | 后台扫描到期监控规则的间隔 |

## 价格来源说明

系统采集的是上游站点实际暴露的价格快照。全局阈值比较使用内置 LiteLLM 格式价格表：

```text
internal/app/resources/model-pricing/model_prices_and_context_window.json
```

同步时不是只比较分组倍率，还会比较输入、输出、缓存读写、请求价格是否低于 `官方价格 × 全局阈值`。

## 自动签到

规则运行时会在登录成功后检测上游签到状态：

| 上游类型 | 行为 |
| --- | --- |
| NewAPI | 调用用户签到接口检测今日是否已签到；未签到时自动签到，并把奖励换算为金额显示在规则列表。 |
| sub2api | 当前不支持签到接口，规则列表显示“不支持签到功能”。 |

签到检测失败不会阻断价格监控，失败原因会显示在规则列表的“签到”列中。

## 邮件通知模板

在系统设置中开启自定义邮件模板后，可以通过模板类型 tab 分别配置不同通知内容：

| 模板类型 | 触发场景 |
| --- | --- |
| 新低价 | 全局价格快照出现新的最低价。 |
| 同步成功 | 低价渠道成功创建或更新到主站 sub2api。 |
| 同步失败 | 主站账号同步失败，例如分组权限、余额或接口错误。 |
| 账号更新 | 主站 sub2api 渠道账号被更新。 |
| 余额不足 | 低价上游余额不足被跳过，只通知低价候选。 |
| 默认兜底 | 某个类型未配置模板时使用的默认模板。 |

模板支持 `{{subject}}`、`{{body}}`、`{{notification_type}}`、`{{site_name}}`、`{{model_name}}`、`{{group_name}}`、`{{group_ratio}}`、`{{upstream_balance}}`、`{{input_price}}`、`{{output_price}}`、`{{cache_read_price}}`、`{{cache_write_price}}`、`{{request_price}}`、`{{error}}` 等变量。

## 推荐社区

推荐关注 [LINUX DO](https://linux.do)，适合交流 AI API、中转站、部署运维和开发实践相关内容。

## 部署文档

完整部署和运维说明见 [docs/deployment.md](docs/deployment.md)。
