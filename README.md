# Flymon

[![Release](https://img.shields.io/github/v/release/iflyelf/flymon)](https://github.com/iflyelf/flymon/releases)
[![Go Version](https://img.shields.io/badge/go-1.25-blue)](https://go.dev)
[![License](https://img.shields.io/github/license/iflyelf/flymon)](LICENSE)

Flymon 是基于 [夜莺（Nightingale）](https://github.com/ccfos/nightingale) v9.0.0+ 的深度定制版，采用 **go-zero 架构包装层**，内置 **事件聚合功能**，可跟随官方自动升级。

## 核心特性

- **go-zero 架构**：使用 go-zero ServiceGroup 管理服务生命周期，统一日志与优雅退出
- **事件聚合**：60 秒聚合窗口，同一规则下的重复告警批量发送，减少告警风暴
- **静态编译**：CGO_ENABLED=0，开箱即用，无需依赖
- **跟随官方升级**：upstream 子模块机制，每 6 小时自动检查官方新版本并 PR
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
```

配置文件格式与官方夜莺完全一致，参考 [官方文档](https://flashcat.cloud/docs/)。

## 与官方夜莺的区别

| 特性 | 夜莺（官方） | Flymon |
|------|-------------|--------|
| 架构 | 原生 Go 标准库 | go-zero 包装层 |
| 事件聚合 | 无 | 内置（60 秒窗口） |
| 编译产物名 | n9e / n9e-edge / n9e-pushgw | flymon / flymon-edge / flymon-pushgw |
| 版本号 | v9.0.0 | v9.0.0-flymon |
| 升级跟随 | - | 自动（GitHub Actions） |

**兼容性**：Flymon 的配置文件、数据库表结构、API 接口均与官方 v9.0.0 保持一致，可平滑迁移。

## 事件聚合说明

### 工作原理

同一告警规则 + 通知规则 + 通知渠道 + 模板组合的事件，在首次触发后进入 60 秒聚合窗口，期间所有新事件追加到缓存，窗口到期后批量发送，减少通知风暴。

### 聚合 Key

```
RuleName|NotifyRuleId|NotifyConfigChannelId|NotifyConfigTemplateId|NotifyChannelId|MessageTemplateId
```

### 实现位置

`upstream/alert/dispatch/dispatch.go`（通过 `apply-aggregation-patch.py` 自动注入）

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

```bash
cd upstream
git fetch --tags
git checkout v9.1.0  # 指定官方新版本
cd ..
git add upstream

# 同步依赖和 SQL
bash scripts/gomod-sync.sh
bash scripts/sync-sql.sh

# 验证编译
bash scripts/build.sh
```

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
│   └── flymon-pushgw/       # 推送网关入口
├── internal/
│   ├── bootstrap/           # go-zero 启动封装
│   ├── cli/                 # 命令行参数解析
│   └── edge/                # edge Initialize（上游在 main 包中需复刻）
├── upstream/                # 子模块：官方 nightingale
├── scripts/
│   ├── build.sh             # 交叉编译脚本
│   ├── gomod-sync.sh        # 依赖同步脚本
│   └── sync-sql.sh          # SQL 同步脚本
├── sql/                     # 数据库初始化文件（自动同步）
├── apply-aggregation-patch.py  # 事件聚合补丁
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
- [go-zero](https://github.com/zeromicro/go-zero) - 微服务框架
