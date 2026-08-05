# Flymon Helm Chart

Flymon 是基于夜莺(Nightingale) v9 的深度定制版，内置动态聚合告警引擎、5个Go化通知媒介、Gateway回调服务。

## 特性

- **动态聚合告警引擎**：可配置窗口/维度/策略的事件聚合，支持 Web 界面实时配置
- **5 个 Go 化内置通知媒介**：飞书、短信、邮件、IM、IM加急（去除 Python 依赖）
- **Gateway 回调服务**：飞书交互卡片、快捷屏蔽、AI 分析、协同群
- **自动建表**：启动时自动 AutoMigrate（含 aggregation_config 表）
- **基于夜莺 v9.0.0**：上游版本锁定 703bb65f

## 快速开始

### 前置条件

- Kubernetes 1.19+
- Helm 3.0+
- MySQL 5.7+ 或 PostgreSQL 9.6+（或使用 SQLite）
- Redis 5.0+（告警引擎依赖）

### 安装

#### 1. 添加 Helm 仓库（如有）

```bash
helm repo add flymon https://iflyelf.github.io/flymon
helm repo update
```

#### 2. 创建 values.yaml

最小化配置（使用默认 MySQL/Redis 地址）：

```yaml
config:
  database:
    dsn: "root:password@tcp(mysql:3306)/n9e_v6?charset=utf8mb4&parseTime=True&loc=Local"
  redis:
    address: "redis:6379"
    password: ""

gateway:
  enabled: true
  config:
    n9eApiToken: "your-n9e-api-token"
    redisAddr: "redis:6379"
```

#### 3. 安装 Chart

```bash
helm install flymon ./flymon-chart -n monitoring --create-namespace -f values.yaml
```

或从仓库安装：

```bash
helm install flymon flymon/flymon -n monitoring --create-namespace -f values.yaml
```

#### 4. 访问 Web UI

```bash
# 端口转发
kubectl port-forward -n monitoring svc/flymon 17000:17000

# 访问 http://localhost:17000
# 默认账号: root / root.2020
```

## 配置说明

### 部署模式

通过 `mode` 参数选择部署模式：

- `center`（默认）：中心服务（Web UI + API + 告警引擎 + 内置媒介）
- `edge`：边缘告警引擎
- `pushgw`：推送网关

### 数据库配置

支持 MySQL / PostgreSQL / SQLite：

```yaml
config:
  database:
    dbType: "mysql"
    dsn: "root:password@tcp(mysql-host:3306)/n9e_v6?charset=utf8mb4&parseTime=True&loc=Local"
    maxIdleConns: 10
    maxOpenConns: 100
```

启动时自动 AutoMigrate 建表，无需手动导入 SQL。

### Redis 配置

支持单机/哨兵/集群：

```yaml
config:
  redis:
    address: "redis:6379"
    redisType: "standalone"  # standalone / cluster / sentinel
    password: ""
    db: 0
```

### Gateway 配置

Gateway 提供飞书交互卡片/AI分析/协同群功能：

```yaml
gateway:
  enabled: true
  replicaCount: 2
  config:
    # N9E API（必填）
    n9eApiUrl: "http://flymon:17000/api/n9e"
    n9eApiToken: "your-token"
    
    # Token 存储：file(单机) / redis(多副本推荐)
    tokenStoreType: "redis"
    redisMode: "standalone"
    redisAddr: "redis:6379"
    
    # AI 分析（可选）
    openclawEnabled: false
    openclawApiUrl: ""
    openclawApiToken: ""
```

#### Gateway Token 存储模式

**file 模式**（单副本）：
```yaml
gateway:
  config:
    tokenStoreType: "file"
  persistence:
    enabled: true
    storageClass: "nfs-client"
    size: 1Gi
```

**redis 模式**（多副本推荐）：
```yaml
gateway:
  config:
    tokenStoreType: "redis"
    redisMode: "cluster"  # standalone / sentinel / cluster
    redisAddr: "redis-cluster-0:6379,redis-cluster-1:6379,redis-cluster-2:6379"
    redisPassword: ""
```

Redis 集群/哨兵配置示例：

```yaml
# 哨兵模式
gateway:
  config:
    redisMode: "sentinel"
    redisAddr: "sentinel-0:26379,sentinel-1:26379,sentinel-2:26379"
    redisMasterName: "mymaster"
    redisPassword: "xxx"

# 集群模式
gateway:
  config:
    redisMode: "cluster"
    redisAddr: "node1:7000,node2:7000,node3:7000"
    redisPassword: "xxx"
```

### 敏感信息保护

生产环境推荐启用 Secret（默认开启）：

```yaml
useSecret: true

config:
  database:
    dsn: "root:password@tcp(mysql:3306)/n9e_v6?..."
  redis:
    password: "redis-password"

gateway:
  config:
    n9eApiToken: "your-token"
    redisPassword: "redis-password"
    openclawApiToken: "ai-token"
```

所有敏感值会存入 Secret 并通过环境变量注入。

### 资源限制

```yaml
resources:
  requests:
    cpu: "1"
    memory: 2Gi
  limits:
    cpu: "4"
    memory: 8Gi

gateway:
  resources:
    requests:
      cpu: 100m
      memory: 64Mi
    limits:
      cpu: 500m
      memory: 256Mi
```

### 自动扩缩容 (HPA)

```yaml
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 80

gateway:
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 6
```

### Ingress

```yaml
ingress:
  enabled: true
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
  hosts:
    - host: flymon.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: flymon-tls
      hosts:
        - flymon.example.com
```

## 使用场景示例

### 场景 1: 单机开发环境

```yaml
mode: center
replicaCount: 1

config:
  database:
    dbType: "sqlite"
    dsn: "/opt/flymon/data/n9e.db"
  redis:
    address: "redis:6379"

gateway:
  enabled: false

persistence:
  data:
    enabled: true
    size: 5Gi

resources:
  requests:
    cpu: 500m
    memory: 512Mi
```

### 场景 2: 生产环境（高可用）

```yaml
mode: center
replicaCount: 3

config:
  database:
    dbType: "mysql"
    dsn: "root:xxx@tcp(mysql-ha:3306)/n9e_v6?..."
    maxOpenConns: 200
  redis:
    address: "redis-ha:6379"
    redisType: "sentinel"
    masterName: "mymaster"
    password: "xxx"

gateway:
  enabled: true
  replicaCount: 3
  config:
    tokenStoreType: "redis"
    redisMode: "sentinel"
    redisAddr: "redis-sentinel-0:26379,redis-sentinel-1:26379"
    redisMasterName: "mymaster"

autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10

podDisruptionBudget:
  enabled: true
  minAvailable: 2

resources:
  requests:
    cpu: "2"
    memory: 4Gi
  limits:
    cpu: "8"
    memory: 16Gi
```

### 场景 3: 边缘告警引擎

```yaml
mode: edge
replicaCount: 2

config:
  http:
    port: 19000
    apiForService:
      enable: true
  redis:
    address: "redis:6379"

gateway:
  enabled: false
```

## 配置完整参数说明

详见 [values.yaml](values.yaml) 内的注释。

## 卸载

```bash
helm uninstall flymon -n monitoring
```

## 常见问题

### Q1: 启动后 Pod CrashLoopBackOff？

检查数据库连接：
```bash
kubectl logs -n monitoring flymon-xxx | grep -i "database\|mysql\|connect"
```

确认 DSN 格式正确，数据库已创建（如 `CREATE DATABASE n9e_v6 CHARACTER SET utf8mb4`）。

### Q2: Gateway 配置校验失败？

报错 `缺少必填环境变量: N9E_API_URL, N9E_API_TOKEN`，检查：
```bash
kubectl get secret flymon -n monitoring -o yaml
# 确认 GATEWAY_N9E_API_TOKEN 是否存在且非空
```

### Q3: 聚合不生效？

1. 检查 Web 界面"聚合配置"是否启用
2. 聚合窗口建议 30-300 秒
3. 绑定规则是否匹配实际触发的告警规则
4. 查看日志：`kubectl logs -n monitoring flymon-xxx | grep AggregationEngine`

### Q4: file 模式 Gateway 多副本数据不同步？

file 模式不支持多副本，需要：
- 改用 `tokenStoreType: redis`（推荐）
- 或使用 RWX 存储（如 NFS、CephFS）

## 更新日志

见 [flymon README](https://github.com/iflyelf/flymon)

## 许可

Apache-2.0
