# Flymon

[![Release](https://img.shields.io/github/v/release/iflyelf/flymon)](https://github.com/iflyelf/flymon/releases)
[![Go Version](https://img.shields.io/badge/go-1.25-blue)](https://go.dev)
[![License](https://img.shields.io/github/license/iflyelf/flymon)](LICENSE)

Flymon 是基于 [夜莺（Nightingale）](https://github.com/ccfos/nightingale) v9.0.0+ 的深度定制版，采用 **go-zero 架构包装层**，内置 **动态聚合告警引擎**、**5 个 Go 化通知媒介**、**Gateway 回调服务**，可跟随官方自动升级。

## 核心特性

- **go-zero 架构**：使用 go-zero ServiceGroup 管理服务生命周期，统一日志与优雅退出
- **动态聚合告警**：可配置窗口（10-600s）、多维度聚合（规则/业务组/级别/数据源/标签）、immediate/delay 策略
- **5 个内置通知媒介**：飞书卡片/短信/邮件/飞书 IM/飞书 IM 加急，Python 脚本 → Go 原生实现，参数 Web 配置
- **Gateway 回调服务**：飞书交互式卡片屏蔽、AI 告警分析（OpenClaw/iflyelf 双引擎）、协同群创建
- **聚合屏蔽**：按聚合维度批量屏蔽一组事件（如同一业务组的所有主机告警）
- **静态编译**：CGO_ENABLED=0，开箱即用，无需依赖
- **跟随官方升级**：upstream 子模块机制，精简 patch（单行替换），最小化升级冲突
- **多平台支持**：linux/amd64、linux/arm64

## 快速开始

### 下载

从 [Releases](https://github.com/iflyelf/flymon/releases) 下载对应平台的压缩包：

```bash
wget https://github.com/iflyelf/flymon/releases/download/v9.0.0-flymon/flymon-v9.0.0-flymon-linux-amd64.tar.gz
tar xzf flymon-v9.0.0-flymon-linux-amd64.tar.gz
cd flymon-v9.0.0-flymon-linux-amd64
```

### 初始化数据库

```bash
# MySQL
mysql -uroot -p n9e_v6 < sql/mysql-schema.sql
mysql -uroot -p n9e_v6 < sql/mysql-init-data.sql

# SQLite
sqlite3 n9e.db < sql/sqlite-schema.sql
```

### 启动服务

```bash
# 中心节点
./flymon --configs=etc

# 边缘节点
./flymon-edge --configs=etc

# 推送网关
./flymon-pushgw --configs=etc

# 告警回调网关（飞书交互卡片/AI 分析/协同群，可选）
./flymon-gateway --configs=etc
```

配置文件格式与官方夜莺完全一致，参考 [官方文档](https://flashcat.cloud/docs/)。

> 数据库表由程序启动时自动迁移（GORM AutoMigrate），新增的 `aggregation_config` 表与 `alert_mute` 聚合屏蔽字段无需手动执行 SQL。

## 与官方夜莺的区别

| 特性 | 夜莺（官方） | Flymon |
|------|-------------|--------|
| 架构 | 原生 Go 标准库 | go-zero 包装层 |
| 事件聚合 | 无 | 动态聚合引擎（窗口/维度/策略可配置） |
| 通知媒介 | 需外挂脚本 | 5 个 Go 化内置媒介（飞书/短信/邮件/IM/IM 加急） |
| 飞书交互回调 | 无 | 内置 Gateway（屏蔽按钮/AI 分析/协同群） |
| 聚合屏蔽 | 无 | 按聚合维度批量屏蔽 |
| 编译产物名 | n9e / n9e-edge / n9e-pushgw | flymon / flymon-edge / flymon-pushgw / flymon-gateway |
| 版本号 | v9.0.0 | v9.0.0-flymon |
| 升级跟随 | - | 自动（GitHub Actions） |

**兼容性**：Flymon 的配置文件、数据库表结构、API 接口均与官方 v9.0.0 保持一致，可平滑迁移。

## 聚合告警说明

### 工作原理

聚合引擎按 Web 端配置的 **聚合配置（AggregationConfig）** 将告警事件归入时间窗口，窗口到期后批量渲染发送，减少通知风暴。无匹配配置时自动回退为直接发送，保证告警不丢失。

### 聚合配置字段

| 字段 | 说明 |
|------|------|
| `window_duration` | 聚合窗口秒数（10-600） |
| `group_by_rule_name/group_name/severity/datasource` | 聚合维度开关 |
| `group_by_tags` | 按指定标签键聚合 |
| `send_strategy` | `delay`（窗口到期批量）/ `immediate`（首个立即+后续批量） |
| `notify_rule_ids` | 绑定的通知规则（空=全局生效） |
| `filter_enable` + `label_filters/severities/datasource_ids` | 过滤适用事件 |

聚合 Key 由所选维度动态拼接，例如勾选「规则名+业务组」生成 `R=cpu-high|G=web`。

### Web 配置入口

- 聚合配置：`监控配置 → 聚合配置`（API：`/api/n9e/aggregation-configs`）
- 消息模板：复用官方 `监控配置 → 消息模板`，模板内用 `{{ range $events }}` 遍历聚合事件

### 实现位置

- 聚合引擎：`upstream/alert/dispatch/aggregation_engine.go`
- 模型/缓存：`upstream/models/aggregation_config.go`、`upstream/memsto/aggregation_config_cache.go`
- 发送 patch：`upstream/alert/dispatch/dispatch.go`（`apply-aggregation-patch.py` 单行替换）

## 内置通知媒介

5 个由 Python 脚本改写的 Go 原生媒介，启动时自动 Upsert 到 `notify_channel` 表，开箱即用。参数遵循 **Web 参数配置 > 环境变量 > 默认值** 优先级。

| 媒介 ident | 说明 | 主要参数（Web 配置） |
|-----------|------|---------------------|
| `feishu_builtin` | 飞书群机器人交互卡片 | `feishu_domain`、`access_token` |
| `sms_builtin` | 短信网关 | `sms_gateway_url`、`sms_account`、`sms_password` |
| `email_builtin` | SMTP 邮件（含 Grafana 图表） | `email_host`、`email_port`、`email_user`、`mail_pass` |
| `im_builtin` | 飞书应用发群消息 | `feishu_app_id`、`feishu_app_secret` |
| `im_urgent_builtin` | 飞书 IM + 电话/短信加急 | 同 IM + 加急参数 |

**环境变量配置**（Grafana / AI / Gateway 全局）：

```bash
GRAFANA_BASE_URL / GRAFANA_TOKEN / GRAFANA_DASHBOARD_UID / GRAFANA_DATASOURCE_UID
CALLBACK_SERVER_URL   # flymon-gateway 地址，飞书屏蔽按钮/AI 分析用
```

实现位置：`upstream/alert/sender/provider/*_builtin_provider.go`

## 告警回调网关（flymon-gateway）

独立子命令，承接飞书交互式卡片的回调：

| 端点 | 说明 |
|------|------|
| `POST /feishu_callback` | 飞书卡片交互（屏蔽按钮/AI/协同群） |
| `POST /ai_analysis/register` + `GET /ai_analysis/trigger` | AI 告警分析 |
| `POST /mute/register` | 屏蔽 token 注册 |
| `POST /group_chat/register` | 协同群 token 注册 |
| `GET /health` | 健康检查 |

Token 存储支持 `file`（默认）与 `redis` 双实现，通过环境变量 `TOKEN_STORE_TYPE` 切换。实现位置：`internal/gateway/`。

## 聚合屏蔽

`alert_mute` 表新增 `aggregated_mute`、`aggregation_group_by` 两列，支持按聚合维度（rule_name/group_name/severity/datasource）批量屏蔽一组事件。匹配逻辑见 `upstream/alert/mute/mute.go:MatchMuteForAggregation`。

## 本地开发

### 环境要求

- Go 1.25+
- Python 3（应用补丁）
- Git（管理子模块）

### 克隆仓库

```bash
git clone --recursive https://github.com/iflyelf/flymon.git
cd flymon
```

### 应用补丁并编译

```bash
# 应用事件聚合补丁
python3 apply-aggregation-patch.py upstream/alert/dispatch/dispatch.go

# 构建（支持交叉编译）
PLATFORMS="linux/amd64 linux/arm64" bash scripts/build.sh

# 产物位于 bin/ 和 dist/
ls -lh bin/
```

### 升级上游版本

**自动升级（推荐）**: GitHub Actions 每 6 小时检查一次官方新版本，发现后自动创建 PR。

**手动升级**: 若需手动升级到指定版本（如 v9.1.0），在 upstream fork 的 `flymon-custom` 分支上执行：

```bash
cd upstream
git checkout flymon-custom

# 添加官方仓库并拉取新 tag
git remote add ccfos https://github.com/ccfos/nightingale.git 2>/dev/null || \
  git remote set-url ccfos https://github.com/ccfos/nightingale.git
git fetch ccfos --tags

# 关键：用 merge 而非 checkout，保留定制代码
git merge v9.1.0  # ← 官方新版本，定制代码会被保留

# 解决冲突（若有），然后推送
git push origin flymon-custom

# 回到主仓库，更新子模块指针和基线 tag
cd ..
echo "v9.1.0" > .upstream-base-tag
git add upstream .upstream-base-tag

# 同步依赖和 SQL
bash scripts/gomod-sync.sh
bash scripts/sync-sql.sh

# 验证编译
bash scripts/build.sh
```

**重要**：绝不能用 `git checkout <tag>` 切换 upstream，这会丢失所有定制代码（聚合引擎/内置媒介等 2347 行）。必须用 `git merge <tag>` 合并新版本。

## 架构设计

### go-zero 包装层

Flymon 采用"包装层"方案而非完整重写：

```
cmd/flymon/main.go (go-zero 启动入口)
    ↓
internal/bootstrap/bootstrap.go (ServiceGroup 生命周期管理)
    ↓
upstream/center/center.go:Initialize() (官方业务逻辑)
```

优势：
- 业务代码直接复用上游，可跟随官方升级
- go-zero 提供统一的服务管理、日志、优雅退出
- 最小化维护成本

### 目录结构

```
flymon/
├── cmd/
│   ├── flymon/              # 中心服务入口
│   ├── flymon-edge/         # 边缘节点入口
│   ├── flymon-pushgw/       # 推送网关入口
│   └── flymon-gateway/      # 告警回调网关入口（飞书交互/AI/协同群）
├── internal/
│   ├── bootstrap/           # go-zero 启动封装
│   ├── cli/                 # 命令行参数解析
│   ├── edge/                # edge Initialize（上游在 main 包中需复刻）
│   └── gateway/             # Gateway 服务实现（config/handlers/ai/feishu/n9e/token/cards）
├── upstream/                # 子模块：官方 nightingale（含聚合/媒介/屏蔽增强）
│   ├── models/aggregation_config.go            # 聚合配置模型
│   ├── memsto/aggregation_config_cache.go      # 聚合配置缓存
│   ├── alert/dispatch/aggregation_engine.go    # 聚合引擎
│   ├── alert/sender/provider/*_builtin_provider.go  # 5 个内置媒介
│   ├── alert/mute/mute.go                       # 聚合屏蔽匹配
│   └── center/router/router_aggregation_config.go   # 聚合配置 Web API
├── scripts/
│   ├── build.sh             # 交叉编译脚本
│   ├── gomod-sync.sh        # 依赖同步脚本
│   └── sync-sql.sh          # SQL 同步脚本
├── sql/                     # 数据库初始化文件（自动同步）
├── apply-aggregation-patch.py  # 事件聚合补丁（精简版，单行替换）
├── go.mod                   # 主模块依赖（含 upstream replace）
└── README.md
```

## GitHub Actions

### Release（release.yml）

- **触发**：推送 v* tag 或手动触发
- **产物**：linux/amd64、linux/arm64 静态二进制 + SQL 文件
- **发布**：自动创建 GitHub Release

### Sync Upstream（sync-upstream.yml）

- **触发**：每 6 小时定时 + 手动触发
- **检查**：对比 upstream 当前版本与官方最新正式版
- **升级**：自动创建 PR，包含子模块更新、依赖同步、SQL 同步

## 依赖管理注意事项

**⚠️ 重要**：不要对 flymon 主模块直接执行 `go mod tidy`。

### 原因

Go modules 不会继承依赖模块（upstream）的 `replace` 指令，而 upstream 依赖若干 fork 版本：
- `golang.org/x/exp` 必须锁定旧版本（prometheus v0.47.1 依赖其旧版 `slices.SortFunc(bool)` 签名）
- `github.com/olivere/elastic/v7` 被 replace 到 n9e fork

直接 tidy 会导致 go 1.26+ 的 MVS 升级 x/exp 到新版本（`slices.SortFunc(int)` 签名），导致编译失败。

### 正确做法

```bash
# 升级上游后同步依赖
bash scripts/gomod-sync.sh
```

该脚本会自动从 `upstream/go.mod` 提取所有 `replace` 指令并合并到主模块。

## 贡献

欢迎提交 Issue 和 PR。如果修改了 upstream 子模块，请同步运行 `scripts/gomod-sync.sh` 和 `scripts/sync-sql.sh`。

## 许可证

与上游 nightingale 保持一致。

## 致谢

- [Nightingale（夜莺）](https://github.com/ccfos/nightingale) - 原项目

---

## 更新日志

### v9.0.0-flymon-aggregation (2025-01)

**重大更新：告警聚合与通知媒介 Go 化**

#### 新增功能
- **动态聚合告警引擎**
  - 可配置窗口（10-600s）、多维度聚合（规则名/业务组/级别/数据源/标签）
  - 支持 `delay`（窗口到期批量）与 `immediate`（首个立即+后续批量）策略
  - 无匹配配置时自动回退直接发送，保证告警不丢失
  - Web 配置：`/api/n9e/aggregation-configs` CRUD 接口
  - 实现：`upstream/alert/dispatch/aggregation_engine.go`（独立文件，精简 patch）

- **5 个内置通知媒介**（Python 脚本 → Go 原生）
  - `feishu_builtin` — 飞书群机器人交互卡片
  - `sms_builtin` — 短信网关
  - `email_builtin` — SMTP 邮件（含 Grafana 图表内嵌）
  - `im_builtin` — 飞书应用群消息
  - `im_urgent_builtin` — 飞书 IM + 电话/短信加急
  - 参数优先级：Web 参数配置 > 环境变量 > 默认值
  - 启动时自动 Upsert 到 `notify_channel` 表，开箱即用
  - 实现：`upstream/alert/sender/provider/*_builtin_provider.go`

- **告警回调网关（flymon-gateway）**
  - 飞书交互式卡片回调（屏蔽按钮、AI 分析、协同群创建）
  - AI 引擎双支持：OpenClaw / iflyelf，带故障转移
  - Token 存储双实现：file（默认）/ redis
  - 独立子命令：`./flymon-gateway --configs=etc`
  - 实现：`internal/gateway/`

- **聚合屏蔽增强**
  - `alert_mute` 表新增 `aggregated_mute`、`aggregation_group_by` 字段
  - 支持按聚合维度（rule_name/group_name/severity/datasource）批量屏蔽一组事件
  - 匹配逻辑：`upstream/alert/mute/mute.go:MatchMuteForAggregation`
  - 单元测试覆盖：`upstream/alert/mute/aggregation_mute_test.go`

#### 数据库变更（自动迁移）
- 新增表：`aggregation_config`（聚合配置）
- 新增列：`alert_mute.aggregated_mute`、`alert_mute.aggregation_group_by`
- 新增媒介：5 个内置通知媒介记录（启动时自动创建）

#### 技术债改进
- 事件聚合 patch 精简为**单行替换**（`dispatch.go:220`），最大化跟随上游升级能力
- 聚合引擎懒加载（全局变量 + sync.Once），无需改动 NewDispatch 签名
- Provider 参数读取统一封装（`notify_config_helper.go`）
- Grafana 渲染与事件聚合去重逻辑复刻 Python 原版（`aggregation_helper.go`）

#### 代码统计
- 新增代码：4409 行 Go
- 新增文件：23 个
- 修改文件：7 个（upstream 5 个、主模块 2 个）
- 测试覆盖：聚合屏蔽 6 个单元测试（100% PASS）

#### 编译产物
- `flymon` (159MB) — 中心节点
- `flymon-gateway` (27MB) — 告警回调网关
- `flymon-pushgw` (108MB) — 推送网关
- `flymon-edge` (99MB) — 边缘节点（未变动）
- [go-zero](https://github.com/zeromicro/go-zero) - 微服务框架
