# Flymon 配置文件

Flymon 配置文件格式与官方夜莺完全一致，请参考：

- [官方文档](https://flashcat.cloud/docs/)
- [上游示例配置](https://github.com/ccfos/nightingale/tree/master/docker)

## 配置文件位置

默认读取 `etc/` 目录下的配置文件，可通过 `--configs` 参数指定：

```bash
./flymon --configs=/path/to/config
```

## 快速配置

参考上游 docker-compose 示例：

```bash
# 从上游复制示例配置
cp -r upstream/docker/compose/etc ./etc-example
cd etc-example
# 根据环境修改数据库、Redis 连接信息
```

## 环境变量

- `N9E_CONFIGS` - flymon 配置目录
- `N9E_EDGE_CONFIGS` - flymon-edge 配置目录
- `N9E_PUSHGW_CONFIGS` - flymon-pushgw 配置目录

## 配置项

与官方夜莺一致，主要包括：

- 数据库连接（MySQL/PostgreSQL/SQLite）
- Redis 连接
- HTTP 服务端口
- 告警引擎配置
- 通知渠道配置
- 数据源配置

详见官方文档。
