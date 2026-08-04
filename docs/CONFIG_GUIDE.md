# Flymon 配置指南

本文档说明 flymon 的配置方式，区分**环境变量配置**（容器启动前设置）和 **Web 界面配置**（运行时在夜莺前端操作）。

---

## 📦 一、环境变量配置（容器启动前）

这些配置通过环境变量或 `docker-compose.yml` 的 `environment` 传入，**服务启动时生效，运行中不可更改**。

### 1.1 必填配置（缺失则启动报错）

| 环境变量 | 说明 | 示例 |
|---------|------|------|
| `N9E_API_URL` | 夜莺 API 地址 | `http://n9e:19000/api/n9e` |
| `N9E_API_TOKEN` | 夜莺 API 认证 Token | `uuid-格式-token` |

### 1.2 Gateway 回调服务（可选，提供飞书交互功能）

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `LISTEN_HOST` | `0.0.0.0` | Gateway 监听 IP |
| `LISTEN_PORT` | `5000` | Gateway 监听端口 |
| `DATA_DIR` | `/data/gateway` | Token 持久化目录（file模式需挂载卷） |
| `TOKEN_STORE_TYPE` | `file` | Token 存储类型：`file`(单机) / `redis`(多副本) |

#### Redis Token 存储（多副本场景）

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `REDIS_ADDR` | 空 | Redis 地址（`host:port`），`TOKEN_STORE_TYPE=redis` 时必填 |
| `REDIS_USERNAME` | 空 | Redis 用户名 |
| `REDIS_PASSWORD` | 空 | Redis 密码 |
| `REDIS_DB` | `0` | Redis 数据库编号 |
| `REDIS_PREFIX` | `n9e_gateway` | 键前缀 |

### 1.3 AI 智能分析（可选）

启用后告警卡片提供"AI 分析"按钮。

#### OpenClaw（龙虾）

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `OPENCLAW_ENABLED` | `false` | 是否启用 OpenClaw |
| `OPENCLAW_API_URL` | 空 | OpenClaw API 地址，启用时必填 |
| `OPENCLAW_API_TOKEN` | 空 | OpenClaw API Token，启用时必填 |
| `OPENCLAW_MODEL` | `openclaw/default` | 模型名称 |

#### iflyelf AI

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `IFLYELF_ENABLED` | `false` | 是否启用 iflyelf AI |
| `IFLYELF_API_URL` | 空 | iflyelf API 地址，启用时必填 |
| `IFLYELF_API_TOKEN` | 空 | iflyelf API Token，启用时必填 |
| `IFLYELF_MODEL` | `auto` | 模型名称 |

#### AI 调度策略

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `AI_PROVIDER_STRATEGY` | `openclaw_first` | 策略：`openclaw_first`、`iflyelf_first`、`openclaw_only`、`iflyelf_only` |
| `AI_TIMEOUT` | `300` | AI 请求超时（秒） |
| `AI_MAX_TOKENS` | `1500` | 最大生成 token 数 |

### 1.4 飞书域名（特殊需求）

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `FEISHU_DOMAIN` | `open.feishu.cn` | 飞书 OpenAPI 域名（私有部署改为自定义域名） |

### 1.5 示例：docker-compose.yml

```yaml
version: '3.8'
services:
  flymon:
    image: iflyelf/flymon:latest
    ports:
      - "19000:19000"  # 夜莺主端口
      - "5000:5000"    # Gateway 回调端口（默认5000，可通过 LISTEN_PORT 修改）
    environment:
      # 必填
      N9E_API_URL: "http://localhost:19000/api/n9e"
      N9E_API_TOKEN: "your-n9e-api-token-here"
      
      # Gateway（多副本用 redis）
      TOKEN_STORE_TYPE: "redis"
      REDIS_ADDR: "redis:6379"
      REDIS_PASSWORD: "your-redis-password"
      
      # AI 分析（可选）
      OPENCLAW_ENABLED: "true"
      OPENCLAW_API_URL: "http://openclaw-api:8790"
      OPENCLAW_API_TOKEN: "your-openclaw-token"
    volumes:
      - ./data:/data
    depends_on:
      - redis
      - mysql
```

---

## 🖥️ 二、Web 界面配置（运行时动态配置）

这些配置在**夜莺 Web 前端**操作，保存到数据库，运行时立即生效。

### 2.1 聚合配置（告警事件聚合规则）

**路径**: 夜莺 Web → 告警管理 → 聚合配置

**配置项**：

| 字段 | 类型 | 说明 |
|------|------|------|
| 名称 | 文本 | 配置名称，便于识别 |
| 说明 | 文本 | 用途描述 |
| 启用状态 | 开关 | 是否启用此配置 |
| 聚合窗口 | 整数 | 时间窗口（10-600 秒），同窗口内事件合并为一条 |
| 聚合维度 | 多选 | 按哪些维度分组：<br>- 告警规则名<br>- 业务组<br>- 严重级别<br>- 数据源<br>- 自定义标签（如 `cluster,service`） |
| 发送策略 | 单选 | `delay` 等窗口结束发送 / `immediate` 窗口内立即发送 |
| 绑定通知规则 | 多选 | 空=全局生效，否则仅对指定规则生效 |
| 过滤条件 | JSON | 标签过滤（只对匹配的告警应用聚合） |

**示例场景**：
- 服务重启导致 100 台机器同时告警 → 聚合为 1 条"100 台主机不可达"
- Kubernetes Pod 频繁重启 → 每 60 秒聚合一次，减少通知轰炸

### 2.2 内置通知媒介配置（5 个开箱即用媒介）

**路径**: 夜莺 Web → 系统管理 → 通知媒介 → 找到以下 5 个内置媒介

#### 2.2.1 飞书 Webhook (`feishu_builtin`)

| 参数名 | 必填 | 说明 |
|--------|------|------|
| `feishu_domain` | 否 | 飞书域名（默认 `open.feishu.cn`，私有部署改为自定义） |
| `domain_url` | 是 | **Gateway 回调地址**，如 `http://gateway:5000` |
| `access_token` | 是 | 飞书 Webhook Token（在飞书群设置→机器人→自定义机器人中获取） |

**用户联系信息**（通知对象配置）：
- 在"用户管理"中填写用户的 `feishu_webhook` 字段（飞书用户的 open_id 或 user_id）

#### 2.2.2 短信 (`sms_builtin`)

| 参数名 | 必填 | 说明 |
|--------|------|------|
| `sms_gateway_url` | 是 | 短信网关 API 地址 |
| `sms_account` | 是 | 短信网关账号 |
| `sms_password` | 是 | 短信网关密码 |

**用户联系信息**：
- 填写用户的 `phone` 字段

#### 2.2.3 邮件 (`email_builtin`)

| 参数名 | 必填 | 说明 |
|--------|------|------|
| `email_host` | 是 | SMTP 主机（如 `smtp.exmail.qq.com`） |
| `email_port` | 是 | SMTP 端口（如 `465`、`587`） |
| `email_user` | 是 | SMTP 登录用户名 |
| `email_from` | 是 | 发件人邮箱 |
| `mail_pass` | 是 | SMTP 密码 |

**用户联系信息**：
- 填写用户的 `email` 字段

#### 2.2.4 IM 群聊 (`im_builtin`)

发送卡片消息到飞书群（支持卡片交互按钮：快捷屏蔽、AI 分析、拉群讨论）。

| 参数名 | 必填 | 说明 |
|--------|------|------|
| `feishu_app_id` | 是 | 飞书企业自建应用的 App ID |
| `feishu_app_secret` | 是 | 飞书应用的 App Secret |
| `feishu_domain` | 否 | 飞书域名（默认 `open.feishu.cn`） |

**用户联系信息**：
- 填写用户的 `feishu_chat_id` 字段（飞书群的 chat_id）

#### 2.2.5 IM 加急 (`im_urgent_builtin`)

在 IM 群聊基础上，触发飞书电话或短信加急（P0 告警专用）。

| 参数名 | 必填 | 说明 |
|--------|------|------|
| `feishu_app_id` | 是 | 飞书应用 App ID |
| `feishu_app_secret` | 是 | 飞书应用 App Secret |
| `feishu_domain` | 否 | 飞书域名 |
| `urgent_type` | 是 | 加急方式：`sms`（短信）/ `phone`（电话） |
| `urgent_user_ids` | 是 | 加急接收人的飞书 user_id，逗号分隔（如 `aaa,bbb,ccc`） |

**用户联系信息**：
- 填写用户的 `feishu_chat_id` 字段

### 2.3 聚合屏蔽（屏蔽维度支持聚合分组键）

**路径**: 夜莺 Web → 告警管理 → 告警屏蔽 → 创建屏蔽规则

**新增字段**：

| 字段 | 说明 |
|------|------|
| 聚合屏蔽 | 开关，启用后按聚合维度屏蔽（屏蔽"某类聚合告警"而非单个原始事件） |
| 聚合维度 | 多选，与聚合配置一致（rule_name、group_name、severity、datasource、自定义标签） |

**应用场景**：
- 屏蔽"所有 Kubernetes 集群的 Pod 重启"（按 `cluster` 和 `namespace` 维度屏蔽）
- 临时屏蔽某个聚合维度的告警噪音，不影响其他维度

---

## 🔄 三、配置生效时间对比

| 配置类型 | 配置方式 | 生效时间 | 是否需重启 |
|---------|---------|---------|-----------|
| 环境变量（Gateway、AI、Redis） | docker-compose / k8s env | 容器重启后生效 | ✅ 需要 |
| 聚合配置 | Web 界面 | 保存后立即生效（后台缓存 9 秒刷新） | ❌ 无需 |
| 内置媒介参数 | Web 界面 | 保存后立即生效 | ❌ 无需 |
| 聚合屏蔽 | Web 界面 | 保存后立即生效 | ❌ 无需 |

---

## 📌 四、快速配置检查清单

### 启动前（环境变量）
- [ ] `N9E_API_URL` 和 `N9E_API_TOKEN` 已设置
- [ ] 多副本部署时配置 `TOKEN_STORE_TYPE=redis` 和 Redis 连接
- [ ] AI 功能需要时配置 `OPENCLAW_*` 或 `IFLYELF_*`

### 启动后（Web 界面）
- [ ] 进入"通知媒介"，找到 5 个 `*_builtin` 媒介，配置参数（SMTP、飞书 App ID/Secret 等）
- [ ] 进入"用户管理"，填写用户联系信息（phone、email、feishu_chat_id 等）
- [ ] 进入"聚合配置"，创建聚合规则（设置窗口、维度、绑定规则）
- [ ] 测试告警发送，观察是否正常聚合和通知

---

## ❓ 五、常见问题

**Q1: 内置媒介找不到？**
- A: 启动日志中搜索 `upsert builtin notify channel`，确认 5 个媒介是否自动注册成功。若失败，检查数据库连接和迁移日志。

**Q2: Gateway 回调失败（飞书交互按钮无响应）？**
- A: 检查 `domain_url` 是否配置为外网可访问地址（飞书服务器需回调此地址）。内网地址需配置飞书 Outbound IP 白名单。

**Q3: Redis 连接失败？**
- A: 启动时报错 `redis 连接失败(addr=...)`，检查 `REDIS_ADDR`、`REDIS_PASSWORD` 是否正确，以及 Redis 服务是否启动。

**Q4: AI 分析按钮无响应？**
- A: 检查 `OPENCLAW_ENABLED` 或 `IFLYELF_ENABLED` 是否为 `true`，以及对应 API URL 和 Token 是否正确。Gateway 日志中搜索 `AI` 查看详细错误。

**Q5: 聚合不生效？**
- A: 检查"聚合配置"是否启用、窗口是否合理（建议 30-300 秒）、绑定规则是否匹配实际触发的告警规则。查看日志中 `AggregationEngine` 相关输出。

---

## 📚 相关文档

- [README.md](../README.md) - 项目概述与核心特性
- [架构设计](../README.md#架构设计) - 聚合引擎工作原理
- [更新日志](../README.md#更新日志) - 版本特性说明
