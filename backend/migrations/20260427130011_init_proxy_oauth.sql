-- +goose Up
-- +goose StatementBegin

-- 代理（HTTP / HTTPS / SOCKS5）
CREATE TABLE IF NOT EXISTS `proxy` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name`          VARCHAR(128) NOT NULL,
  `protocol`      VARCHAR(16)  NOT NULL                COMMENT 'http / https / socks5 / socks5h',
  `host`          VARCHAR(255) NOT NULL,
  `port`          INT UNSIGNED NOT NULL,
  `username`      VARCHAR(255) DEFAULT NULL,
  `password_enc`  BLOB         DEFAULT NULL            COMMENT 'AES-256-GCM 加密',
  `status`        TINYINT      NOT NULL DEFAULT 1      COMMENT '1启用 0停用',
  `last_check_at` DATETIME(3)  DEFAULT NULL,
  `last_check_ok` TINYINT      NOT NULL DEFAULT 0      COMMENT '0未知 1OK 2失败',
  `last_check_ms` INT          NOT NULL DEFAULT 0,
  `last_error`    VARCHAR(255) DEFAULT NULL,
  `remark`        VARCHAR(255) DEFAULT NULL,
  `created_by`    BIGINT UNSIGNED DEFAULT NULL,
  `created_at`    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`    DATETIME(3)  DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_status` (`status`),
  KEY `idx_deleted` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='出站代理池';

-- account 表扩展：proxy_id + OAuth 元数据 + 测试结果
ALTER TABLE `account`
  ADD COLUMN `proxy_id`               BIGINT UNSIGNED DEFAULT NULL AFTER `base_url`,
  ADD COLUMN `oauth_meta`              JSON         DEFAULT NULL AFTER `credential_enc`     COMMENT '非敏感 OAuth 元数据：email / chatgpt_account_id / plan_type 等',
  ADD COLUMN `access_token_enc`        BLOB         DEFAULT NULL                            COMMENT 'AES-256-GCM 加密的 access_token',
  ADD COLUMN `refresh_token_enc`       BLOB         DEFAULT NULL                            COMMENT 'AES-256-GCM 加密的 refresh_token',
  ADD COLUMN `access_token_expires_at` DATETIME(3)  DEFAULT NULL                            COMMENT 'access_token 失效时间',
  ADD COLUMN `last_refresh_at`         DATETIME(3)  DEFAULT NULL                            COMMENT '最近一次成功刷新 RT 时间',
  ADD COLUMN `last_test_at`            DATETIME(3)  DEFAULT NULL                            COMMENT '最近一次连通性测试时间',
  ADD COLUMN `last_test_status`        TINYINT      NOT NULL DEFAULT 0                      COMMENT '0未测 1OK 2失败',
  ADD COLUMN `last_test_latency_ms`    INT          NOT NULL DEFAULT 0,
  ADD COLUMN `last_test_error`         VARCHAR(255) DEFAULT NULL,
  ADD KEY `idx_account_proxy` (`proxy_id`),
  ADD KEY `idx_account_token_exp` (`access_token_expires_at`);

-- 系统配置：代理 + OAuth 默认
INSERT INTO `system_config` (`key`, `value`, `remark`) VALUES
  ('proxy.global_enabled',          'false', '是否启用全局代理'),
  ('proxy.global_id',               '0',     '全局默认代理 ID（0 表示不使用）'),
  ('oauth.refresh_before_hours',    '6',     'access_token 距过期 N 小时内自动刷新'),
  ('oauth.openai_client_id',        '"app_EMoamEEZ73f0CkXaXp7hrann"', 'OpenAI Codex CLI 公开 client_id'),
  ('oauth.openai_token_url',        '"https://auth.openai.com/oauth/token"', 'OpenAI OAuth Token Endpoint')
ON DUPLICATE KEY UPDATE `value`=VALUES(`value`);

-- +goose StatementEnd

-- +goose Down
ALTER TABLE `account`
  DROP COLUMN `proxy_id`,
  DROP COLUMN `oauth_meta`,
  DROP COLUMN `access_token_enc`,
  DROP COLUMN `refresh_token_enc`,
  DROP COLUMN `access_token_expires_at`,
  DROP COLUMN `last_refresh_at`,
  DROP COLUMN `last_test_at`,
  DROP COLUMN `last_test_status`,
  DROP COLUMN `last_test_latency_ms`,
  DROP COLUMN `last_test_error`;
DROP TABLE IF EXISTS `proxy`;
DELETE FROM `system_config` WHERE `key` IN (
  'proxy.global_enabled', 'proxy.global_id',
  'oauth.refresh_before_hours', 'oauth.openai_client_id', 'oauth.openai_token_url'
);
