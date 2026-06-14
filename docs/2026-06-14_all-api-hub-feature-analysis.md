# all-api-hub 功能借鉴分析

日期：2026-06-14

## 结论

`all-api-hub` 对本项目最有价值的不是整体 UI 框架，而是模型数据源分层、API 模型探测、请求封装和模型列表交互。当前项目已经合并了第一阶段能力：在系统设置中新增 API 模型探测面板，支持 OpenAI 兼容和 Anthropic `/v1/models` 读取、搜索、分页和复制模型 ID。

## 参考项目重点

参考仓库：

```text
https://github.com/qixing-jk/all-api-hub.git
```

已重点查看的方向：

| 方向 | all-api-hub 做法 | 对本项目价值 |
| --- | --- | --- |
| 模型价格查询 | 将站点价格、用户可用模型、完整模型目录分层处理 | 避免把 `/v1/models` 错当成完整价格源 |
| API 请求 | OpenAI 兼容、Anthropic、Google 等协议单独封装 | 本项目可扩展更多 API 类型的模型探测 |
| UI 排版 | 模型列表支持搜索、过滤、状态提示、分页 | 适合迁移到设置页和快照列表 |
| 能力边界 | 不支持的站点能力会在 UI 上隐藏或提示 | 避免用户误以为某个模型一定可用或可同步 |

## 已合并内容

### API 模型探测后端

新增文件：

```text
internal/app/model_probe.go
internal/app/model_probe_test.go
```

新增接口：

```http
POST /api/model-probe
```

请求示例：

```json
{
  "api_type": "openai_compatible",
  "base_url": "https://api.example.com/v1",
  "api_key": "sk-..."
}
```

支持的 API 类型：

| 类型 | 请求接口 | 认证方式 |
| --- | --- | --- |
| `openai_compatible` | `GET /v1/models` | `Authorization: Bearer <key>` |
| `anthropic` | `GET /v1/models?limit=200` | `x-api-key` + `anthropic-version` |

实现细节：

- Base URL 已包含 `/v1` 时，不会重复拼接成 `/v1/v1/models`。
- Anthropic 支持分页读取。
- 结果按模型 ID 去重并排序。
- API Key 只用于本次探测，不写入数据库。

### API 模型探测前端

新增位置：

```text
系统设置 -> API 模型探测
```

交互能力：

- 可选择已保存站点自动填充 Base URL。
- 可手动填写 Base URL 和 API Key。
- 可切换 OpenAI 兼容或 Anthropic。
- 查询结果支持关键词搜索、分页、复制模型 ID。
- 页面文案明确说明：探测结果只代表当前 API Key 可见模型，不作为完整价格目录。

### 文档更新

已更新：

```text
README.md
```

增加了 API 模型探测功能说明、请求方式和适用范围。

## 暂未合并但值得继续做的功能

### 1. 模型列表能力标签

all-api-hub 会在模型列表中展示模型能力，例如 Provider、模式、上下文、输出限制、端点类型等。

建议迁移到本项目：

- sub2api 用户价格列表增加能力标签列。
- 价格快照列表增加模型能力摘要。
- 对支持 `openai`、`anthropic` 的模型显示不同徽章。

预期收益：

- 快速区分同名模型在不同站点是否支持 OpenAI 或 Anthropic 调用。
- 辅助判断 Claude 模型被错误归到 Codex 分类的问题。

### 2. 价格源状态提示

all-api-hub 对“完整目录”和“用户可用模型”做清晰区分。

建议迁移到本项目：

- 在 sub2api 用户价格区域显示数据来源状态。
- 区分“站点价格接口返回”、“官方价格表补全”、“API 探测结果”。
- 如果只拿到 `/v1/models`，明确提示“不含价格信息”。

预期收益：

- 避免误把模型存在当成价格可同步。
- 排查价格缺失时更直观。

### 3. API 请求统一封装

all-api-hub 把认证头、URL 拼接、错误解析、请求限流放在统一 transport 中。

本项目当前已有部分封装：

- NewAPI 客户端：`internal/app/newapi.go`
- sub2api 客户端：`internal/app/sub2api.go`
- 错误中文化：`internal/app/error_localize.go`

建议后续抽取：

```text
internal/app/http_client.go
```

可统一处理：

- Base URL 归一化
- JSON 请求与响应解析
- 中文错误包装
- 超时与重试
- 敏感头脱敏日志

### 4. 模型探测结果联动规则表单

当前 API 探测只展示模型列表。下一步可以把探测结果联动到规则创建：

- 点击“使用此模型”自动填入监控规则完整模型名。
- 从模型探测表直接创建规则。
- 按 API 类型建议分类：OpenAI 兼容默认 Codex，Anthropic 默认 Claude。

预期收益：

- 减少手动复制模型名。
- 避免模型名输入错误导致无价格快照。

### 5. UI 排版继续优化

all-api-hub 的模型列表更偏“目录浏览”，本项目是“运维后台”。建议保持当前密集表格风格，但可以继续借鉴：

- 顶部状态条展示当前数据来源、模型数量、过滤后数量。
- 表格列支持更细的字段隐藏。
- 复制按钮、快捷填入按钮、能力徽章统一样式。

## 不建议迁移的内容

| 内容 | 原因 |
| --- | --- |
| 整体 React/扩展架构 | 当前项目是 Go 内嵌 HTML/JS，迁移成本高且收益低 |
| 浏览器扩展 runtime messaging | 本项目是服务端 Web 后台，不需要扩展通信层 |
| AIHubMix 专用适配 | 当前核心需求是 NewAPI/sub2api 价格监控，AIHubMix 可作为后续站点类型扩展 |
| `/v1/models` 作为价格源 | 该接口只表示 API Key 可见模型，通常不包含分组倍率和实际价格 |

## 当前验证结果

已执行：

```bash
go test ./...
go build ./...
docker compose -p newapi_price_monitor -f compose.yml up -d --build app
curl -4 -fsS http://127.0.0.1:28080/healthz
```

验证结果：

- Go 单元测试通过。
- Go 构建通过。
- Docker 容器重建并启动成功。
- 健康检查返回 `{"status":"ok"}`。

## 后续建议顺序

1. 给 API 探测表增加“使用此模型创建规则”。
2. 给价格快照和 sub2api 用户价格列表增加模型能力标签。
3. 给不同价格源增加状态提示。
4. 抽取统一 HTTP 请求封装。
5. 如有需要，再增加 AIHubMix 或其他站点专用适配。
