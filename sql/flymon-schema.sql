-- ============================================================================
-- Flymon 定制表结构（MySQL）
--
-- 本文件包含 flymon 在官方夜莺基础上新增的表与列，与官方 SQL 分离维护，
-- 不会被 scripts/sync-sql.sh 覆盖。程序启动时也会通过 GORM AutoMigrate 自动创建，
-- 本文件供全新导入或人工审计使用。
--
-- 使用：
--   mysql -uroot -p n9e_v6 < sql/flymon-schema.sql
-- ============================================================================

-- 聚合配置表：定义告警事件的聚合窗口、维度与发送策略
CREATE TABLE IF NOT EXISTS `aggregation_config` (
    `id` bigint unsigned NOT NULL AUTO_INCREMENT,
    `name` varchar(255) NOT NULL DEFAULT '' COMMENT '聚合配置名称',
    `description` varchar(1024) NOT NULL DEFAULT '' COMMENT '说明',
    `enable` tinyint(1) NOT NULL DEFAULT 1 COMMENT '是否启用',
    `window_duration` int NOT NULL DEFAULT 60 COMMENT '聚合窗口秒数(10-600)',
    `group_by_rule_name` tinyint(1) NOT NULL DEFAULT 0 COMMENT '按告警规则名聚合',
    `group_by_group_name` tinyint(1) NOT NULL DEFAULT 0 COMMENT '按业务组聚合',
    `group_by_severity` tinyint(1) NOT NULL DEFAULT 0 COMMENT '按告警级别聚合',
    `group_by_datasource` tinyint(1) NOT NULL DEFAULT 0 COMMENT '按数据源聚合',
    `group_by_tags` varchar(1024) NOT NULL DEFAULT '[]' COMMENT '按指定标签键聚合(JSON数组)',
    `send_strategy` varchar(32) NOT NULL DEFAULT 'delay' COMMENT 'delay|immediate',
    `notify_rule_ids` varchar(1024) NOT NULL DEFAULT '[]' COMMENT '绑定的通知规则ID(JSON数组,空=全局)',
    `filter_enable` tinyint(1) NOT NULL DEFAULT 0 COMMENT '是否启用过滤',
    `label_filters` text COMMENT '标签过滤条件(JSON)',
    `severities` varchar(255) NOT NULL DEFAULT '[]' COMMENT '适用级别(JSON数组)',
    `datasource_ids` varchar(1024) NOT NULL DEFAULT '[]' COMMENT '适用数据源(JSON数组)',
    `create_at` bigint NOT NULL DEFAULT 0,
    `create_by` varchar(64) NOT NULL DEFAULT '',
    `update_at` bigint NOT NULL DEFAULT 0,
    `update_by` varchar(64) NOT NULL DEFAULT '',
    PRIMARY KEY (`id`),
    KEY `idx_enable` (`enable`),
    KEY `idx_update_at` (`update_at`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COMMENT = 'flymon 聚合配置';

-- 告警屏蔽表：新增聚合屏蔽字段（已有 alert_mute 表时执行）
-- 若列已存在会报错，可忽略；AutoMigrate 会自动处理。
ALTER TABLE `alert_mute`
    ADD COLUMN `aggregated_mute` tinyint(1) NOT NULL DEFAULT 0 COMMENT '0-普通屏蔽 1-聚合屏蔽';
ALTER TABLE `alert_mute`
    ADD COLUMN `aggregation_group_by` varchar(255) NOT NULL DEFAULT '[]' COMMENT '聚合屏蔽维度(JSON数组)';
