package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func localizedHTTPError(provider, endpoint string, status int, body []byte) error {
	message := localizedAPIResponseMessage(status, body)
	if message == "" {
		message = http.StatusText(status)
	}
	return fmt.Errorf("%s %s 返回 HTTP %d：%s", provider, endpoint, status, message)
}

func localizedAPIResponseMessage(status int, body []byte) string {
	text := extractAPIErrorMessage(body)
	if text == "" {
		text = strings.TrimSpace(string(body))
	}
	text = localizeErrorText(text)
	if text == "" {
		text = http.StatusText(status)
	}
	return text
}

func extractAPIErrorMessage(body []byte) string {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return ""
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return raw
	}
	return strings.TrimSpace(extractAPIErrorMessageValue(value))
}

func extractAPIErrorMessageValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		for _, key := range []string{"message", "detail", "error_description", "reason", "code", "status"} {
			if text := stringFromAny(typed[key]); text != "" && !strings.EqualFold(text, "error") {
				return text
			}
		}
		for _, key := range []string{"error", "data", "response"} {
			if text := extractAPIErrorMessageValue(typed[key]); text != "" {
				return text
			}
		}
	case []any:
		for _, item := range typed {
			if text := extractAPIErrorMessageValue(item); text != "" {
				return text
			}
		}
	}
	return ""
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%.0f", typed)
		}
		return fmt.Sprintf("%g", typed)
	}
	return ""
}

func localizeErrorText(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	replacements := []struct {
		old string
		new string
	}{
		{"sub2api sync is disabled", "主站 sub2api 同步开关未开启"},
		{"sub2api main base url is not configured", "主站 sub2api 地址未配置"},
		{"sub2api admin key is not configured", "主站 sub2api 管理员 key 未配置"},
		{"main sub2api admin auth failed", "主站 sub2api 管理员认证失败"},
		{"use the admin API key generated in sub2api admin settings (sent as x-api-key), or paste a valid admin JWT as Bearer token", "请使用 sub2api 管理后台生成的管理员 API Key，或填写有效的管理员 JWT"},
		{"not current available cheapest:", "不是当前可同步最低价："},
		{"not current cheapest:", "不是当前最低价："},
		{"not current cheapest", "不是当前最低价"},
		{"authentication required", "请先登录后台"},
		{"invalid username or password", "账号或密码错误"},
		{"username, current password and new password are required", "账号、当前密码和新密码不能为空"},
		{"new password must be at least 6 characters", "新密码至少需要 6 个字符"},
		{"current password is incorrect", "当前密码不正确"},
		{"hash password failed", "密码加密失败"},
		{"save admin credentials failed", "保存管理员账号失败"},
		{"postgres unavailable", "PostgreSQL 不可用"},
		{"load settings failed", "加载设置失败"},
		{"list sites failed", "读取站点列表失败"},
		{"list sub2api upstreams failed", "读取 sub2api 上游站点失败"},
		{"list categories failed", "读取分类失败"},
		{"list rules failed", "读取监控规则失败"},
		{"list snapshots failed", "读取价格快照失败"},
		{"source_type must be all, newapi or sub2api", "来源类型只能是 all、newapi 或 sub2api"},
		{"category is required", "分类不能为空"},
		{"model keyword is required", "模型名不能为空"},
		{"invalid rule id", "监控规则 ID 无效"},
		{"invalid site id", "站点 ID 无效"},
		{"invalid category id", "分类 ID 无效"},
		{"invalid sub2api upstream id", "sub2api 上游站点 ID 无效"},
		{"invalid sub2api account id", "sub2api 账号 ID 无效"},
		{"sub2api upstream base url is required", "sub2api 上游地址不能为空"},
		{"sub2api user username/password or auth token is required", "sub2api 用户名密码或 Auth Token 不能为空"},
		{"sub2api access token or email/password is required", "sub2api 访问令牌或邮箱密码不能为空"},
		{"sub2api account requires 2FA code", "sub2api 账号需要二次验证码"},
		{"sub2api login response did not include access_token", "sub2api 登录成功但响应中没有 access_token"},
		{"account requires 2FA; provide a current code or use a monitor account without 2FA", "账号需要二次验证码，请填写当前验证码或使用未开启二次验证的监控账号"},
		{"login succeeded but response did not include user id", "登录成功但响应中没有用户 ID"},
		{"system access token is empty", "系统访问令牌为空"},
		{"token name and group are required", "令牌名称和分组不能为空"},
		{"newapi token key is empty", "NewAPI 令牌 key 为空"},
		{"upstream returned success=false", "上游接口返回失败"},
		{"decode response data", "解析接口 data 失败"},
		{"decode response", "解析接口响应失败"},
		{"decode pricing response", "解析价格响应失败"},
		{"decode sub2api response", "解析 sub2api 响应失败"},
		{"decode sub2api data", "解析 sub2api data 失败"},
		{"api key is required", "API Key 不能为空"},
		{"stagger rules failed", "错峰规则失败"},
		{"restore rule after manual run failed", "手动运行后恢复规则状态失败"},
		{"sub2api account id is required", "sub2api 账号 ID 不能为空"},
		{"sub2api returned code", "sub2api 返回错误码"},
		{"HTTP 400", "HTTP 400（请求参数错误）"},
		{"HTTP 401", "HTTP 401（认证失败）"},
		{"HTTP 403", "HTTP 403（无权限）"},
		{"HTTP 404", "HTTP 404（接口或资源不存在）"},
		{"HTTP 409", "HTTP 409（数据冲突）"},
		{"HTTP 422", "HTTP 422（参数无法处理）"},
		{"HTTP 429", "HTTP 429（上游临时限流）"},
		{"HTTP 500", "HTTP 500（上游内部错误）"},
		{"HTTP 502", "HTTP 502（上游网关错误）"},
		{"HTTP 503", "HTTP 503（上游服务不可用）"},
		{"API returned 400", "接口返回 400"},
		{"API returned 401", "接口返回 401"},
		{"API returned 403", "接口返回 403"},
		{"API returned 404", "接口返回 404"},
		{"API returned 429", "接口返回 429"},
		{"API returned 500", "接口返回 500"},
		{"API returned 502", "接口返回 502"},
		{"API returned 503", "接口返回 503"},
		{"error code: 502", "错误码 502"},
		{"Bad Gateway", "网关错误"},
		{"bad gateway", "网关错误"},
		{"Service temporarily unavailable", "服务暂时不可用"},
		{"service temporarily unavailable", "服务暂时不可用"},
		{"too many requests", "请求过于频繁"},
		{"Too Many Requests", "请求过于频繁"},
		{"rate limit", "限流"},
		{"RATE_LIMIT_EXCEEDED", "请求频率超限"},
		{"MODEL_CAPACITY_EXHAUSTED", "模型容量已满"},
		{"RESOURCE_EXHAUSTED", "资源额度已耗尽"},
		{"QUOTA_EXHAUSTED", "额度已耗尽"},
		{"TLS handshake timeout", "TLS 握手超时"},
		{"tls handshake timeout", "TLS 握手超时"},
		{"timeout", "超时"},
		{"EOF", "上游连接中断"},
		{"connection reset", "连接被重置"},
		{"connection refused", "连接被拒绝"},
		{"test main sub2api account", "测试主站 sub2api 账号"},
		{"sub2api account test did not report success", "主站账号测试没有返回成功结果"},
		{"test failed", "测试失败"},
		{"No available channel for model", "没有可用渠道支持模型"},
		{"no available channel for model", "没有可用渠道支持模型"},
		{"model_not_found", "模型未找到"},
		{"under group", "上游低价分组"},
		{"distributor", "分销商"},
		{"candidate", "候选"},
		{"create NewAPI key", "创建 NewAPI key"},
		{"create newapi token", "创建 NewAPI 令牌"},
		{"create sub2api key", "创建 sub2api key"},
		{"create sub2api account", "创建 sub2api 账号"},
		{"update sub2api account", "更新 sub2api 账号"},
		{"set sub2api account", "设置 sub2api 账号"},
		{"disable duplicate sub2api account", "停用重复 sub2api 账号"},
		{"auth NewAPI upstream", "认证 NewAPI 上游"},
		{"login NewAPI upstream", "登录 NewAPI 上游"},
		{"login sub2api 2fa", "sub2api 二次验证登录"},
		{"login sub2api", "登录 sub2api"},
		{"generate NewAPI system token", "生成 NewAPI 系统 token"},
		{"get newapi token key batch", "批量获取 NewAPI 令牌 key"},
		{"get newapi token key", "获取 NewAPI 令牌 key"},
		{"list newapi tokens", "读取 NewAPI 令牌列表"},
		{"search newapi token", "搜索 NewAPI 令牌"},
		{"token key", "令牌 key"},
		{"upstream", "上游"},
		{"request ", "请求 "},
		{"returned HTTP", "返回 HTTP"},
		{"Invalid URL", "接口地址无效"},
		{"invalid url", "接口地址无效"},
		{"official price not found for model", "官方价格未找到模型"},
		{"INVALID_TOKEN", "令牌无效"},
		{"Invalid token", "令牌无效"},
		{"invalid token", "令牌无效"},
		{"TOKEN_EXPIRED", "令牌已过期"},
		{"Token has expired", "令牌已过期"},
		{"TOKEN_REVOKED", "令牌已撤销"},
		{"INVALID_AUTH_HEADER", "认证头格式无效"},
		{"Authorization header is required", "缺少 Authorization 认证头"},
		{"Authorization header format must be 'Bearer {token}'", "Authorization 认证头格式必须是 Bearer token"},
		{"EMPTY_TOKEN", "令牌不能为空"},
		{"USER_NOT_FOUND", "用户不存在"},
		{"User not found", "用户不存在"},
		{"Invalid API key", "API Key 无效"},
		{"INVALID_API_KEY", "API Key 无效"},
		{"INSUFFICIENT_BALANCE", "账户余额不足"},
		{"Insufficient account balance", "账户余额不足"},
		{"insufficient balance", "余额不足"},
		{"GROUP_DELETED", "分组已删除"},
		{"GROUP_DISABLED", "分组已停用"},
		{"GROUP_NOT_FOUND", "分组不存在"},
		{"group not found", "分组不存在"},
		{"GROUP_NOT_ACTIVE", "目标分组未启用"},
		{"target group is not active", "目标分组未启用"},
		{"INVALID_GROUP_ID", "分组 ID 无效"},
		{"group_id must be non-negative", "分组 ID 不能为负数"},
		{"GROUP_NOT_EXCLUSIVE", "目标分组不是独占分组"},
		{"GROUP_IS_SUBSCRIPTION", "订阅分组不支持该操作"},
		{"SAME_GROUP", "新旧分组不能相同"},
		{"CHANNEL_NOT_FOUND", "渠道不存在"},
		{"channel not found", "渠道不存在"},
		{"CHANNEL_EXISTS", "渠道名称已存在"},
		{"ACCOUNT_NOT_FOUND", "账号不存在"},
		{"account not found", "账号不存在"},
		{"INVALID_CREDENTIALS", "账号或密码错误"},
		{"invalid email or password", "邮箱或密码错误"},
		{"USER_NOT_ACTIVE", "用户未启用"},
		{"user is not active", "用户未启用"},
		{"EMAIL_VERIFY_REQUIRED", "需要邮箱验证"},
		{"EMAIL_SUFFIX_NOT_ALLOWED", "邮箱后缀不允许注册"},
		{"REGISTRATION_DISABLED", "注册已关闭"},
		{"INVITATION_CODE_REQUIRED", "需要邀请码"},
		{"INVITATION_CODE_INVALID", "邀请码无效或已使用"},
		{"TOTP_INVALID_CODE", "二次验证码无效"},
		{"invalid totp code", "二次验证码无效"},
		{"TOTP_TOO_MANY_ATTEMPTS", "二次验证尝试次数过多，请稍后再试"},
		{"PASSWORD_REQUIRED", "密码不能为空"},
		{"password is required", "密码不能为空"},
		{"PAYMENT_DISABLED", "支付系统未开启"},
		{"BALANCE_PAYMENT_DISABLED", "余额充值已关闭"},
		{"INVALID_AMOUNT", "充值金额无效"},
		{"amount must be a positive number", "金额必须大于 0"},
		{"amount out of range", "金额超出允许范围"},
		{"TOO_MANY_PENDING", "待支付订单过多"},
		{"DAILY_LIMIT_EXCEEDED", "已达到今日限制"},
		{"NO_AVAILABLE_INSTANCE", "没有可用支付实例"},
		{"not found", "未找到"},
		{"Not Found", "未找到"},
		{"unauthorized", "未授权"},
		{"Unauthorized", "未授权"},
		{"forbidden", "无权限"},
		{"Forbidden", "无权限"},
		{"permission", "权限不足"},
		{"unsupported", "不支持"},
		{"does not support", "不支持"},
		{"is required", "不能为空"},
		{"failed", "失败"},
	}
	for _, replacement := range replacements {
		text = strings.ReplaceAll(text, replacement.old, replacement.new)
	}
	return text
}
