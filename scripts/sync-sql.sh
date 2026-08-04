#!/usr/bin/env bash
# flymon 数据库初始化脚本
#
# 用途：从 upstream 同步最新的数据库初始化 SQL 文件。
# 这样 flymon 发布时自动包含官方最新的表结构，支持跟随官方升级。

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "🔍 同步 upstream 数据库 SQL 文件..."

# 备份 flymon 定制 SQL（不随 upstream 覆盖）
TMP_KEEP="$(mktemp -d)"
for f in sql/flymon-schema.sql sql/flymon-schema-sqlite.sql sql/flymon-schema-postgres.sql; do
    [[ -f "$f" ]] && cp -v "$f" "$TMP_KEEP/"
done

# 清理并重建 sql/ 目录
rm -rf sql
mkdir -p sql

# 恢复 flymon 定制 SQL
if compgen -G "$TMP_KEEP/*.sql" > /dev/null; then
    cp -v "$TMP_KEEP"/*.sql sql/
fi
rm -rf "$TMP_KEEP"

# 复制 MySQL 初始化 SQL
cp -v upstream/docker/initsql/a-n9e.sql sql/mysql-schema.sql
cp -v upstream/docker/initsql/c-init.sql sql/mysql-init-data.sql

# 复制 SQLite SQL
cp -v upstream/docker/sqlite.sql sql/sqlite-schema.sql

# 复制迁移 SQL（用于升级）
cp -v upstream/docker/migratesql/migrate.sql sql/migrate.sql

# PostgreSQL SQL
if [[ -d upstream/docker/compose-postgres/initsql_for_postgres ]]; then
    cp -v upstream/docker/compose-postgres/initsql_for_postgres/a-n9e-for-Postgres.sql sql/postgres-schema.sql 2>/dev/null || true
    cp -v upstream/docker/compose-postgres/initsql_for_postgres/b-ibex-for-Postgres.sql sql/postgres-ibex.sql 2>/dev/null || true
fi

# 创建使用说明
cat > sql/README.md << 'EOF'
# Flymon 数据库初始化文件

本目录包含从上游 nightingale 同步的数据库初始化脚本（官方一致）以及 flymon 定制表 DDL。

## 官方表结构（由 sync-sql.sh 同步）
- `mysql-schema.sql` / `mysql-init-data.sql` — MySQL 表结构与初始数据
- `sqlite-schema.sql` — SQLite 表结构
- `postgres-schema.sql` / `postgres-ibex.sql` — PostgreSQL 表结构
- `migrate.sql` — 升级迁移 SQL

## Flymon 定制表结构（独立维护，不随 upstream 覆盖）
- `flymon-schema.sql` — MySQL：aggregation_config 表 + alert_mute 聚合屏蔽列
- `flymon-schema-sqlite.sql` — SQLite 定制
- `flymon-schema-postgres.sql` — PostgreSQL 定制

> 程序启动时通过 GORM AutoMigrate 自动创建定制表与新列，无需手动执行。定制 SQL 供全新导入或审计使用。

## MySQL

```bash
mysql -uroot -p n9e_v6 < sql/mysql-schema.sql
mysql -uroot -p n9e_v6 < sql/mysql-init-data.sql
mysql -uroot -p n9e_v6 < sql/flymon-schema.sql   # 可选：flymon 定制表
```

## SQLite

```bash
sqlite3 n9e.db < sql/sqlite-schema.sql
sqlite3 n9e.db < sql/flymon-schema-sqlite.sql    # 可选
```

## PostgreSQL

```bash
psql -U postgres -d n9e_v6 -f sql/postgres-schema.sql
psql -U postgres -d n9e_v6 -f sql/postgres-ibex.sql          # 如需 Ibex 功能
psql -U postgres -d n9e_v6 -f sql/flymon-schema-postgres.sql # 可选
```

## 升级

若从旧版本升级，执行 `migrate.sql` 中对应版本段的 SQL。

## 自动同步

flymon 升级 upstream 子模块后，运行 `scripts/sync-sql.sh` 即可自动同步最新 SQL（会保留 flymon-*.sql 定制文件）。
EOF

echo ""
echo "✅ SQL 文件同步完成，输出到 sql/ 目录"
ls -lh sql/
