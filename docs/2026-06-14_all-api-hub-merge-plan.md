# all-api-hub 功能合并实施计划

日期：2026-06-14

来源文档：

```text
docs/2026-06-14_all-api-hub-feature-analysis.md
```

## 目标

在不迁移整体技术栈的前提下，把 `all-api-hub` 中适合当前价格监控后台的能力逐步合并进来，重点增强模型查询、API 探测、价格源状态、模型能力标签和 UI 操作效率。

## 实施原则

- 保持当前 Go Web + 内嵌 HTML/JS 架构。
- 不把 `/v1/models` 当成价格源，只作为 API Key 可见模型探测。
- 每个阶段都要能独立测试、独立回滚。
- 所有新增用户可见提示使用中文。
- 不保存临时 API Key，除非用户明确在站点账号表单中保存。

## 阶段 0：已完成基线

状态：已完成。

已完成内容：

- 新增 `/api/model-probe`。
- 支持 OpenAI 兼容 `/v1/models`。
- 支持 Anthropic `/v1/models` 分页。
- 设置页新增“API 模型探测”面板。
- 探测结果支持搜索、分页、复制模型 ID。
- README 增加功能说明。

验收证据：

```bash
go test ./...
go build ./...
docker compose -p newapi_price_monitor -f compose.yml up -d --build app
curl -4 -fsS http://127.0.0.1:28080/healthz
```

## 阶段 1：API 探测结果联动规则表单

优先级：高。

状态：已完成。

目标：

让用户在 API 探测结果中直接把模型名带入监控规则，减少复制和手动输入错误。

任务：

1. 在 API 探测结果表增加“使用此模型”按钮。
2. 点击后自动跳转到“添加监控规则”区域。
3. 自动填充 `完整模型名`。
4. 根据 API 类型建议分类：
   - OpenAI 兼容默认选 Codex。
   - Anthropic 默认选 Claude。
5. 如果用户选择了已保存站点，自动同步规则来源和站点选择。
6. 保留手动编辑能力，不自动提交规则。

涉及文件：

```text
internal/app/assets.go
```

验收标准：

- API 探测表中的模型点击后，规则表单模型名正确填入。
- OpenAI 兼容探测默认选中 Codex 分类。
- Anthropic 探测默认选中 Claude 分类。
- 如果分类不存在，不报错，保留当前分类。
- 不会自动保存规则。

测试建议：

```bash
go test ./...
go build ./...
```

浏览器检查：

- 打开系统设置。
- 使用 API 模型探测得到模型列表。
- 点击“使用此模型”。
- 确认规则表单被填充。

## 阶段 2：模型能力标签

优先级：高。

目标：

把模型的 Provider、模式、端点类型、上下文能力等信息展示到价格相关列表中，辅助判断分类和同步是否正确。

任务：

1. 扩展 sub2api 用户价格列表显示能力标签：
   - Provider
   - Mode
   - OpenAI / Anthropic 支持端点
   - 最大输入 tokens
   - 最大输出 tokens
2. 价格快照列表增加模型能力摘要。
3. 对 OpenAI、Anthropic、Gemini 等端点使用不同徽章。
4. 没有能力数据时显示“能力未知”，不要误导用户。

涉及文件：

```text
internal/app/assets.go
internal/app/models.go
internal/app/sub2api_prices.go
```

可能需要检查：

```text
Sub2APIUserPriceRow
PriceSnapshot
PricingRow
```

验收标准：

- sub2api 用户价格表能看到能力标签。
- 价格快照表能看到模型能力摘要。
- 没有能力字段的数据不会显示错误标签。
- 移动端不因新增标签导致表格内容重叠。

测试建议：

```bash
go test ./...
go build ./...
```

浏览器检查：

- 桌面宽度查看 sub2api 用户价格列表。
- 手机宽度查看价格快照列表。
- 确认标签换行正常。

## 阶段 3：价格源状态提示

优先级：中。

目标：

清晰区分不同数据来源，避免把 API 探测模型误认为可同步低价价格。

任务：

1. 在 sub2api 用户价格区域显示数据来源状态：
   - 站点分组倍率
   - 官方价格库
   - 用户覆盖倍率
   - API 探测模型列表
2. 在 API 探测结果中强调“不含价格信息”。
3. 在价格快照列表中标识：
   - 价格来自上游价格接口
   - 官方价格参与阈值计算
   - 快照已失效
4. 对接口失败展示中文状态，不展示原始英文堆栈。

涉及文件：

```text
internal/app/assets.go
internal/app/sub2api_prices.go
internal/app/error_localize.go
```

验收标准：

- 用户能从页面上区分“模型可见”和“价格可同步”。
- `/v1/models` 探测结果不会显示价格列。
- 价格快照依然按已有价格字段排序，不受探测结果影响。

测试建议：

```bash
go test ./...
go build ./...
```

## 阶段 4：统一 HTTP 请求封装

优先级：中。

目标：

减少 NewAPI、sub2api、模型探测中的重复请求逻辑，为后续站点适配打基础。

任务：

1. 新增统一请求文件：

```text
internal/app/http_client.go
```

2. 抽取公共能力：
   - Base URL 归一化
   - URL 拼接，兼容已有 `/v1`
   - JSON 请求
   - JSON 响应解析
   - 非 2xx 中文错误包装
   - User-Agent
   - 超时设置
3. 先迁移模型探测请求。
4. 再小步迁移 NewAPI 或 sub2api 的低风险请求。
5. 避免一次性重构所有客户端。

涉及文件：

```text
internal/app/http_client.go
internal/app/model_probe.go
internal/app/newapi.go
internal/app/sub2api.go
internal/app/error_localize.go
```

验收标准：

- 模型探测测试保持通过。
- NewAPI 登录、价格获取、sub2api 管理接口行为不变。
- 中文错误提示不倒退。

测试建议：

```bash
go test ./...
go build ./...
```

如果迁移了运行路径，还需要 Docker 验证：

```bash
docker compose -p newapi_price_monitor -f compose.yml up -d --build app
curl -4 -fsS http://127.0.0.1:28080/healthz
```

## 阶段 5：可选站点适配扩展

优先级：低。

目标：

在 NewAPI/sub2api 稳定后，再考虑对 AIHubMix 或其他站点增加专用模型目录适配。

任务：

1. 评估是否真的需要 AIHubMix。
2. 如果需要，按 all-api-hub 的分层方式设计：
   - 完整目录层
   - 用户可用模型层
   - fallback 状态
3. 不创建假的 NewAPI 分组。
4. 不把 AIHubMix 的模型目录直接纳入低价同步。

验收标准：

- 新站点适配不影响现有 NewAPI/sub2api 价格排名。
- 用户可用模型和完整目录有明确 UI 区分。
- 不支持的功能在 UI 中隐藏或标记不支持。

## 风险与控制

| 风险 | 控制方式 |
| --- | --- |
| 把模型探测结果误用为价格源 | UI 明确标注“不含价格信息”，后端不写入快照 |
| UI 表格过宽 | 新增字段优先用徽章和折行，移动端验证 |
| 请求封装重构影响现有同步 | 阶段 4 小步迁移，先迁移模型探测 |
| 分类自动选择错误 | 阶段 1 只做建议填充，不自动提交 |
| 临时 API Key 泄露 | 不入库，不写日志，不回显 |

## 建议执行顺序

1. 阶段 1：API 探测结果联动规则表单。
2. 阶段 2：模型能力标签。
3. 阶段 3：价格源状态提示。
4. 阶段 4：统一 HTTP 请求封装。
5. 阶段 5：可选站点适配扩展。

## 每阶段完成定义

每个阶段完成前必须满足：

- 相关代码已实现。
- `go test ./...` 通过。
- `go build ./...` 通过。
- 如果改动影响运行路径，Docker 重建并健康检查通过。
- README 或 docs 已更新。
- Git 提交信息使用中文。
