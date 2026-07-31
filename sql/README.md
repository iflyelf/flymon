# Flymon 数据库初始化文件

本目录包含从上游 nightingale 同步的数据库初始化脚本，与官方保持一致。

## MySQL

1. 执行 `mysql-schema.sql` 创建表结构
2. 执行 `mysql-init-data.sql` 初始化基础数据

```bash
mysql -uroot -p n9e_v6 < sql/mysql-schema.sql
mysql -uroot -p n9e_v6 < sql/mysql-init-data.sql
```

## SQLite

```bash
sqlite3 n9e.db < sql/sqlite-schema.sql
```

## PostgreSQL

```bash
psql -U postgres -d n9e_v6 -f sql/postgres-schema.sql
psql -U postgres -d n9e_v6 -f sql/postgres-ibex.sql  # 如需 Ibex 功能
```

## 升级

若从旧版本升级，执行 `migrate.sql` 中对应版本段的 SQL。

## 自动同步

flymon 升级 upstream 子模块后，运行 `scripts/sync-sql.sh` 即可自动同步最新 SQL。
