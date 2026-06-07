# 部署文档

## 前置要求

- Docker 24+
- Docker Compose v2
- 能访问待监控的 NewAPI/sub2api 上游站点
- 如果主站 sub2api 在宿主机运行，容器内使用 `http://host.docker.internal:<端口>` 访问

## 生产部署

1. 复制环境变量模板。

```bash
cp .env.example .env
```

2. 修改 `.env`。

```env
APP_PORT=28080
POSTGRES_PASSWORD=replace-with-strong-password
BASIC_AUTH_USER=admin
BASIC_AUTH_PASS=replace-with-strong-password
SESSION_SECRET=replace-with-random-secret
MONITOR_INTERVAL=1m
```

3. 启动服务。

```bash
docker compose up -d --build
```

4. 检查状态。

```bash
docker compose ps
docker compose logs --tail=100 app
```

5. 打开后台。

```text
http://服务器IP:28080
```

## 初始化配置

1. 登录后台。
2. 在“系统设置”中配置主站 sub2api 地址和管理员 API Key。
3. 开启“主站 sub2api 同步”。
4. 设置“全局同步低价阈值倍率”，例如 `0.05`。
5. 如需通知，配置 SMTP Host、端口、加密方式、账号密码、发件人和收件人。
6. 添加 NewAPI 或 sub2api 上游站点。
7. 创建监控规则，选择分类、模型关键词、站点和定时间隔。

## SMTP 加密方式

邮件通知支持四种模式：

| 模式 | 行为 |
| --- | --- |
| 自动 | 465 端口使用 SSL/TLS，其它端口尝试 STARTTLS |
| SSL/TLS | 强制 SMTPS |
| STARTTLS | 强制 STARTTLS，不支持则发送失败 |
| 不加密 | 明文 SMTP，不升级 TLS |

## 数据持久化

PostgreSQL 数据保存在 Docker volume：

```text
newapi_price_monitor_pgdata
```

备份示例：

```bash
docker compose exec -T db pg_dump -U postgres newapi_price_monitor > backup.sql
```

恢复示例：

```bash
docker compose exec -T db psql -U postgres newapi_price_monitor < backup.sql
```

## 更新版本

```bash
git pull
docker compose up -d --build
```

应用启动时会自动执行数据库迁移。

## 常见问题

### 容器访问不到宿主机主站 sub2api

在系统设置中把地址写成：

```text
http://host.docker.internal:18080
```

Compose 已配置：

```yaml
extra_hosts:
  - "host.docker.internal:host-gateway"
```

### 阈值等于分组倍率但没有同步

同步阈值不是只比较分组倍率。系统会用官方模型价格乘阈值，分别比较输入、输出、缓存读写、请求价格。任一实际价格高于阈值都会跳过同步，并在规则同步状态中显示原因。

### 手动运行规则是否会同步

会。点击运行一次会执行完整监控逻辑，包括抓取快照、全局低价判断、阈值判断和主站同步。只有满足同步条件时才会写入主站。
