-- ============================================================================
-- Flymon 定制表结构（PostgreSQL）
--
-- 使用：
--   psql -U postgres -d n9e_v6 -f sql/flymon-schema-postgres.sql
-- ============================================================================

CREATE TABLE IF NOT EXISTS aggregation_config (
    id bigserial PRIMARY KEY,
    name varchar(255) NOT NULL DEFAULT '',
    description varchar(1024) NOT NULL DEFAULT '',
    enable smallint NOT NULL DEFAULT 1,
    window_duration int NOT NULL DEFAULT 60,
    group_by_rule_name smallint NOT NULL DEFAULT 0,
    group_by_group_name smallint NOT NULL DEFAULT 0,
    group_by_severity smallint NOT NULL DEFAULT 0,
    group_by_datasource smallint NOT NULL DEFAULT 0,
    group_by_tags varchar(1024) NOT NULL DEFAULT '[]',
    send_strategy varchar(32) NOT NULL DEFAULT 'delay',
    notify_rule_ids varchar(1024) NOT NULL DEFAULT '[]',
    filter_enable smallint NOT NULL DEFAULT 0,
    label_filters text,
    severities varchar(255) NOT NULL DEFAULT '[]',
    datasource_ids varchar(1024) NOT NULL DEFAULT '[]',
    create_at bigint NOT NULL DEFAULT 0,
    create_by varchar(64) NOT NULL DEFAULT '',
    update_at bigint NOT NULL DEFAULT 0,
    update_by varchar(64) NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_aggregation_config_enable ON aggregation_config (enable);
CREATE INDEX IF NOT EXISTS idx_aggregation_config_update_at ON aggregation_config (update_at);

ALTER TABLE alert_mute ADD COLUMN IF NOT EXISTS aggregated_mute smallint NOT NULL DEFAULT 0;
ALTER TABLE alert_mute ADD COLUMN IF NOT EXISTS aggregation_group_by varchar(255) NOT NULL DEFAULT '[]';
