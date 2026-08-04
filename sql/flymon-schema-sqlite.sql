-- ============================================================================
-- Flymon 定制表结构（SQLite）
--
-- 使用：
--   sqlite3 n9e.db < sql/flymon-schema-sqlite.sql
-- ============================================================================

CREATE TABLE IF NOT EXISTS `aggregation_config` (
    `id` INTEGER PRIMARY KEY AUTOINCREMENT,
    `name` varchar(255) NOT NULL DEFAULT '',
    `description` varchar(1024) NOT NULL DEFAULT '',
    `enable` tinyint(1) NOT NULL DEFAULT 1,
    `window_duration` int NOT NULL DEFAULT 60,
    `group_by_rule_name` tinyint(1) NOT NULL DEFAULT 0,
    `group_by_group_name` tinyint(1) NOT NULL DEFAULT 0,
    `group_by_severity` tinyint(1) NOT NULL DEFAULT 0,
    `group_by_datasource` tinyint(1) NOT NULL DEFAULT 0,
    `group_by_tags` varchar(1024) NOT NULL DEFAULT '[]',
    `send_strategy` varchar(32) NOT NULL DEFAULT 'delay',
    `notify_rule_ids` varchar(1024) NOT NULL DEFAULT '[]',
    `filter_enable` tinyint(1) NOT NULL DEFAULT 0,
    `label_filters` text,
    `severities` varchar(255) NOT NULL DEFAULT '[]',
    `datasource_ids` varchar(1024) NOT NULL DEFAULT '[]',
    `create_at` bigint NOT NULL DEFAULT 0,
    `create_by` varchar(64) NOT NULL DEFAULT '',
    `update_at` bigint NOT NULL DEFAULT 0,
    `update_by` varchar(64) NOT NULL DEFAULT ''
);

-- SQLite 不支持 IF NOT EXISTS 的 ADD COLUMN，若列已存在会报错，可忽略。
ALTER TABLE `alert_mute` ADD COLUMN `aggregated_mute` tinyint(1) NOT NULL DEFAULT 0;
ALTER TABLE `alert_mute` ADD COLUMN `aggregation_group_by` varchar(255) NOT NULL DEFAULT '[]';
