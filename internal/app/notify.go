package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type priceChange struct {
	Label string
	Old   string
	New   string
}

func (s *Server) notifyPriceChange(ctx context.Context, previous PriceSnapshot, current PriceSnapshot, changes []priceChange) {
	if len(changes) == 0 {
		return
	}
	settings, err := s.store.GetIntegrationSettings(ctx)
	if err != nil {
		log.Printf("load email settings for price notification: %v", err)
		return
	}
	if !settings.EmailNotifyEnabled || !settings.EmailNotifyPriceChange {
		return
	}

	subject := fmt.Sprintf("[新低价] %s / %s / %s", current.SiteName, current.ModelName, current.GroupName)
	var body strings.Builder
	body.WriteString("价格快照列表检测到新的最低价。\n\n")
	body.WriteString("站点: " + current.SiteName + "\n")
	body.WriteString("地址: " + current.SiteBaseURL + "\n")
	body.WriteString("分类: " + current.CategoryName + "\n")
	body.WriteString("关键词: " + current.ModelKeyword + "\n")
	body.WriteString("模型: " + current.ModelName + "\n")
	body.WriteString("当前分组: " + current.GroupName + "\n")
	if previous.ID > 0 {
		body.WriteString("上一最低价快照: " + previous.CreatedAt.Format(time.RFC3339) + "\n")
	} else {
		body.WriteString("上一最低价快照: 无\n")
	}
	body.WriteString("当前快照: " + current.CreatedAt.Format(time.RFC3339) + "\n\n")
	body.WriteString("变化:\n")
	for _, change := range changes {
		body.WriteString(fmt.Sprintf("- %s: %s -> %s\n", change.Label, change.Old, change.New))
	}
	s.sendEmailAsync(settings, subject, body.String())
}

func (s *Server) notifySyncUpdate(ctx context.Context, rule Rule, site Site, snapshot PriceSnapshot, action string, account sub2Account) {
	settings, err := s.store.GetIntegrationSettings(ctx)
	if err != nil {
		log.Printf("load email settings for sync notification: %v", err)
		return
	}
	if !settings.EmailNotifyEnabled || !settings.EmailNotifySyncUpdate {
		return
	}

	subject := fmt.Sprintf("[主站账号同步] %s %s %s", action, site.Name, snapshot.GroupName)
	s.sendEmailAsync(settings, subject, syncUpdateEmailBody(rule, site, snapshot, action, account))
}

func syncUpdateEmailBody(rule Rule, site Site, snapshot PriceSnapshot, action string, account sub2Account) string {
	var body strings.Builder
	body.WriteString("主站 sub2api 渠道账号已同步。\n\n")
	body.WriteString(fmt.Sprintf("站点: %s\n", site.Name))
	body.WriteString(fmt.Sprintf("地址: %s\n", site.BaseURL))
	body.WriteString(fmt.Sprintf("规则: #%d %s\n", rule.ID, rule.ModelKeyword))
	body.WriteString(fmt.Sprintf("上游账号: %s\n", firstNonEmpty(snapshot.SourceAccount, rule.SourceAccount, "未获取")))
	body.WriteString(fmt.Sprintf("上游账户余额: %s\n", formatBalance(snapshot.UpstreamBalance, snapshot.BalanceUnit)))
	body.WriteString(fmt.Sprintf("模型: %s\n", snapshot.ModelName))
	body.WriteString(fmt.Sprintf("最低价分组: %s\n", snapshot.GroupName))
	body.WriteString(fmt.Sprintf("分组倍率: %s\n", formatFloatPtr(snapshot.GroupRatio)))
	body.WriteString(formatSnapshotPriceLines(snapshot, ""))
	body.WriteString(fmt.Sprintf("主站分组: %s\n", firstNonEmpty(rule.Sub2APIGroupName, rule.CategoryName)))
	body.WriteString(fmt.Sprintf("动作: %s\n", action))
	body.WriteString(fmt.Sprintf("主站账号: #%d %s\n", account.ID, account.Name))
	body.WriteString("分组倍率变动明细:\n")
	body.WriteString(fmt.Sprintf("- 上游最低价分组: %s，倍率 %s\n", snapshot.GroupName, formatFloatPtr(snapshot.GroupRatio)))
	body.WriteString(fmt.Sprintf("- 主站账号倍率: %s\n", formatFloatPtr(account.Rate)))
	body.WriteString(fmt.Sprintf("- 同步动作: %s\n", action))
	return body.String()
}

func (s *Server) notifySyncFailure(ctx context.Context, rule Rule, site Site, row PricingRow, err error) {
	settings, loadErr := s.store.GetIntegrationSettings(ctx)
	if loadErr != nil {
		log.Printf("load email settings for sync failure notification: %v", loadErr)
		return
	}
	if !settings.EmailNotifyEnabled || !settings.EmailNotifySyncUpdate {
		return
	}

	subject := fmt.Sprintf("[主站账号同步失败] %s / %s", site.Name, row.GroupName)
	body := fmt.Sprintf(
		"主站 sub2api 渠道账号同步失败。\n\n站点: %s\n地址: %s\n规则: #%d %s\n上游账号: %s\n模型: %s\n最低价分组: %s\n错误: %v\n",
		site.Name,
		site.BaseURL,
		rule.ID,
		rule.ModelKeyword,
		firstNonEmpty(rule.Sub2APIUpstreamName, "未绑定"),
		row.ModelName,
		row.GroupName,
		err,
	)
	s.sendEmailAsync(settings, subject, body)
}

func (s *Server) notifySub2APIAccountUpdate(ctx context.Context, action string, account sub2Account) {
	settings, err := s.store.GetIntegrationSettings(ctx)
	if err != nil {
		log.Printf("load email settings for account notification: %v", err)
		return
	}
	if !settings.EmailNotifyEnabled || !settings.EmailNotifySyncUpdate {
		return
	}

	baseURL := stringMapValue(account.Credentials, "base_url")
	groups := make([]string, 0, len(account.Groups))
	for _, group := range account.Groups {
		groups = append(groups, fmt.Sprintf("%s #%d", group.Name, group.ID))
	}
	if len(groups) == 0 {
		for _, id := range account.GroupIDs {
			groups = append(groups, fmt.Sprintf("#%d", id))
		}
	}

	subject := fmt.Sprintf("[主站账号更新] %s #%d %s", action, account.ID, account.Name)
	body := fmt.Sprintf(
		"主站 sub2api 渠道账号已更新。\n\n动作: %s\n账号: #%d %s\napiurl: %s\n平台/类型: %s / %s\n分组: %s\n状态: %s\n调度: %t\n",
		action,
		account.ID,
		account.Name,
		baseURL,
		account.Platform,
		account.Type,
		strings.Join(groups, ", "),
		account.Status,
		account.Schedulable,
	)
	s.sendEmailAsync(settings, subject, body)
}

func (s *Server) notifyLowBalanceSkip(ctx context.Context, rule Rule, skipped []PriceSnapshot, candidate PriceSnapshot) {
	if len(skipped) == 0 {
		return
	}
	settings, err := s.store.GetIntegrationSettings(ctx)
	if err != nil {
		log.Printf("load email settings for low balance notification: %v", err)
		return
	}
	if !settings.EmailNotifyEnabled || !settings.EmailNotifySyncUpdate {
		return
	}

	subject := fmt.Sprintf("[低价上游余额不足] %s / %s", skipped[0].SiteName, skipped[0].ModelName)
	var body strings.Builder
	body.WriteString("最低价上游账号余额不足，已跳过该低价渠道同步。\n\n")
	body.WriteString(fmt.Sprintf("规则: #%d %s\n", rule.ID, rule.ModelKeyword))
	body.WriteString("分类: " + firstNonEmpty(rule.CategoryName, rule.Category) + "\n")
	body.WriteString("模型: " + skipped[0].ModelName + "\n\n")
	body.WriteString("被跳过的低价上游:\n")
	for _, snapshot := range skipped {
		body.WriteString(fmt.Sprintf("- 站点: %s\n", snapshot.SiteName))
		body.WriteString(fmt.Sprintf("  地址: %s\n", snapshot.SiteBaseURL))
		if account := strings.TrimSpace(snapshot.SourceAccount); account != "" {
			body.WriteString(fmt.Sprintf("  登录账号: %s\n", account))
		}
		body.WriteString(fmt.Sprintf("  分组: %s\n", snapshot.GroupName))
		body.WriteString(fmt.Sprintf("  分组倍率: %s\n", formatFloatPtr(snapshot.GroupRatio)))
		body.WriteString(formatSnapshotPriceLines(snapshot, "  "))
		body.WriteString(fmt.Sprintf("  余额: %s\n", formatBalance(snapshot.UpstreamBalance, snapshot.BalanceUnit)))
	}
	if candidate.ID > 0 && !snapshotBalanceInsufficient(candidate) {
		body.WriteString("\n当前可同步候选:\n")
		body.WriteString(fmt.Sprintf("站点: %s\n地址: %s\n", candidate.SiteName, candidate.SiteBaseURL))
		if account := strings.TrimSpace(candidate.SourceAccount); account != "" {
			body.WriteString(fmt.Sprintf("登录账号: %s\n", account))
		}
		body.WriteString(fmt.Sprintf("分组: %s\n分组倍率: %s\n", candidate.GroupName, formatFloatPtr(candidate.GroupRatio)))
		body.WriteString(formatSnapshotPriceLines(candidate, ""))
		body.WriteString(fmt.Sprintf("余额: %s\n", formatBalance(candidate.UpstreamBalance, candidate.BalanceUnit)))
	}
	s.sendEmailAsync(settings, subject, body.String())
}

func snapshotPriceChanges(previous PriceSnapshot, current PriceSnapshot) []priceChange {
	changes := make([]priceChange, 0, 7)
	if previous.GroupName != current.GroupName {
		changes = append(changes, priceChange{Label: "最低价分组", Old: previous.GroupName, New: current.GroupName})
	}
	addFloatChange := func(label string, oldValue *float64, newValue *float64) {
		if !sameFloatPtr(oldValue, newValue) {
			changes = append(changes, priceChange{Label: label, Old: formatFloatPtr(oldValue), New: formatFloatPtr(newValue)})
		}
	}
	addFloatChange("分组倍率", previous.GroupRatio, current.GroupRatio)
	addFloatChange("输入价格", previous.InputPrice, current.InputPrice)
	addFloatChange("输出价格", previous.OutputPrice, current.OutputPrice)
	addFloatChange("缓存读价格", previous.CacheReadPrice, current.CacheReadPrice)
	addFloatChange("缓存写价格", previous.CacheWritePrice, current.CacheWritePrice)
	addFloatChange("请求价格", previous.RequestPrice, current.RequestPrice)
	return changes
}

func lowestSnapshotChanges(previous PriceSnapshot, current PriceSnapshot) []priceChange {
	if previous.ID == 0 {
		return []priceChange{{
			Label: "最低价渠道",
			Old:   "无",
			New:   lowestSnapshotLabel(current),
		}}
	}
	changes := make([]priceChange, 0, 9)
	if !sameLowestSnapshotSource(previous, current) {
		changes = append(changes, priceChange{
			Label: "最低价渠道",
			Old:   lowestSnapshotLabel(previous),
			New:   lowestSnapshotLabel(current),
		})
	}
	changes = append(changes, snapshotPriceChanges(previous, current)...)
	return changes
}

func sameLowestSnapshot(previous PriceSnapshot, current PriceSnapshot) bool {
	return len(lowestSnapshotChanges(previous, current)) == 0
}

func sameLowestSnapshotSource(previous PriceSnapshot, current PriceSnapshot) bool {
	if !strings.EqualFold(strings.TrimSpace(previous.SourceType), strings.TrimSpace(current.SourceType)) {
		return false
	}
	if previous.SiteID != current.SiteID || previous.Sub2APIUpstreamID != current.Sub2APIUpstreamID {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(previous.SiteBaseURL), strings.TrimSpace(current.SiteBaseURL)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(previous.SourceAccount), strings.TrimSpace(current.SourceAccount)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(previous.ModelName), strings.TrimSpace(current.ModelName)) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(previous.GroupName), strings.TrimSpace(current.GroupName))
}

func lowestSnapshotLabel(snapshot PriceSnapshot) string {
	parts := []string{}
	if sourceType := strings.TrimSpace(snapshot.SourceType); sourceType != "" {
		parts = append(parts, sourceType)
	}
	if siteName := strings.TrimSpace(snapshot.SiteName); siteName != "" {
		parts = append(parts, siteName)
	}
	if account := strings.TrimSpace(snapshot.SourceAccount); account != "" {
		parts = append(parts, "账号 "+account)
	}
	if groupName := strings.TrimSpace(snapshot.GroupName); groupName != "" {
		parts = append(parts, groupName)
	}
	if len(parts) == 0 {
		return "未知"
	}
	return strings.Join(parts, " / ")
}

func sameFloatPtr(left *float64, right *float64) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func formatFloatPtr(value *float64) string {
	if value == nil {
		return "空"
	}
	return strconv.FormatFloat(*value, 'g', 10, 64)
}

func firstPricePtr(snapshot PriceSnapshot) *float64 {
	if snapshot.InputPrice != nil {
		return snapshot.InputPrice
	}
	if snapshot.RequestPrice != nil {
		return snapshot.RequestPrice
	}
	return snapshot.OutputPrice
}

func formatSnapshotPriceLines(snapshot PriceSnapshot, prefix string) string {
	var body strings.Builder
	body.WriteString(fmt.Sprintf("%s输入价格: %s\n", prefix, formatFloatPtr(snapshot.InputPrice)))
	body.WriteString(fmt.Sprintf("%s输出价格: %s\n", prefix, formatFloatPtr(snapshot.OutputPrice)))
	body.WriteString(fmt.Sprintf("%s缓存读价格: %s\n", prefix, formatFloatPtr(snapshot.CacheReadPrice)))
	body.WriteString(fmt.Sprintf("%s缓存写价格: %s\n", prefix, formatFloatPtr(snapshot.CacheWritePrice)))
	body.WriteString(fmt.Sprintf("%s请求价格: %s\n", prefix, formatFloatPtr(snapshot.RequestPrice)))
	return body.String()
}

func formatBalance(value *float64, unit string) string {
	if value == nil {
		return "未知"
	}
	unit = strings.ToLower(strings.TrimSpace(unit))
	switch unit {
	case "":
		return strconv.FormatFloat(*value, 'g', 10, 64)
	case "usd":
		return "$" + strconv.FormatFloat(*value, 'f', 6, 64)
	case "quota":
		return "$" + strconv.FormatFloat(newAPIQuotaToUSD(*value), 'f', 6, 64)
	case "balance":
		return "$" + strconv.FormatFloat(*value, 'f', 6, 64)
	default:
		return strconv.FormatFloat(*value, 'g', 10, 64) + " " + unit
	}
}

func (s *Server) sendEmailAsync(settings IntegrationSettings, subject string, body string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := sendEmail(ctx, settings, subject, body); err != nil {
			log.Printf("send email notification: %v", err)
		}
	}()
}

func sendEmail(ctx context.Context, settings IntegrationSettings, subject string, body string) error {
	host := strings.TrimSpace(settings.SMTPHost)
	if host == "" || settings.SMTPPort <= 0 {
		return errors.New("smtp host or port is not configured")
	}
	from := strings.TrimSpace(settings.SMTPFrom)
	if from == "" {
		return errors.New("smtp from is not configured")
	}
	recipients, err := parseEmailList(settings.SMTPTo)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return errors.New("smtp recipients are not configured")
	}

	fromAddress, err := mail.ParseAddress(from)
	if err != nil {
		return fmt.Errorf("invalid smtp from %q: %w", from, err)
	}

	addr := net.JoinHostPort(host, strconv.Itoa(settings.SMTPPort))
	headers := map[string]string{
		"From":         from,
		"To":           strings.Join(recipients, ", "),
		"Subject":      mimeHeader(subject),
		"MIME-Version": "1.0",
		"Content-Type": `text/plain; charset="UTF-8"`,
	}
	var message strings.Builder
	for key, value := range headers {
		message.WriteString(key + ": " + value + "\r\n")
	}
	message.WriteString("\r\n")
	message.WriteString(body)

	auth := smtp.Auth(nil)
	if settings.SMTPUsername != "" || settings.SMTPPassword != "" {
		auth = smtp.PlainAuth("", settings.SMTPUsername, settings.SMTPPassword, host)
	}

	switch normalizeSMTPEncryption(settings.SMTPEncryption) {
	case "ssl":
		return sendSMTPS(ctx, addr, auth, fromAddress.Address, recipients, []byte(message.String()), host)
	case "starttls":
		return sendSMTP(ctx, addr, auth, fromAddress.Address, recipients, []byte(message.String()), host, true, true)
	case "plain":
		return sendSMTP(ctx, addr, auth, fromAddress.Address, recipients, []byte(message.String()), host, false, false)
	default:
		if settings.SMTPPort == 465 {
			return sendSMTPS(ctx, addr, auth, fromAddress.Address, recipients, []byte(message.String()), host)
		}
		return sendSMTP(ctx, addr, auth, fromAddress.Address, recipients, []byte(message.String()), host, true, false)
	}
}

func sendSMTP(ctx context.Context, addr string, auth smtp.Auth, from string, recipients []string, message []byte, serverName string, allowSTARTTLS bool, requireSTARTTLS bool) error {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, serverName)
	if err != nil {
		return err
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); allowSTARTTLS && ok {
		if err := client.StartTLS(&tls.Config{ServerName: serverName}); err != nil {
			return err
		}
	} else if requireSTARTTLS {
		return errors.New("smtp server does not support STARTTLS")
	}
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	return sendSMTPMessage(client, from, recipients, message)
}

func sendSMTPS(ctx context.Context, addr string, auth smtp.Auth, from string, recipients []string, message []byte, serverName string) error {
	dialer := &tls.Dialer{Config: &tls.Config{ServerName: serverName}}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, serverName)
	if err != nil {
		return err
	}
	defer client.Close()
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	return sendSMTPMessage(client, from, recipients, message)
}

func sendSMTPMessage(client *smtp.Client, from string, recipients []string, message []byte) error {
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func parseEmailList(value string) ([]string, error) {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r'
	})
	recipients := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		address, err := mail.ParseAddress(part)
		if err != nil {
			return nil, fmt.Errorf("invalid recipient %q: %w", part, err)
		}
		recipients = append(recipients, address.Address)
	}
	return recipients, nil
}

func mimeHeader(value string) string {
	return mime.QEncoding.Encode("UTF-8", value)
}
