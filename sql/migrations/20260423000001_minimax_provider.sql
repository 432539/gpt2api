-- +goose Up
-- +goose StatementBegin

-- ============================================================
-- 2026-04-23 Add MiniMax-M2.7 LLM support
--
-- Changes:
--   1. Add `provider` column to `models` table so the gateway
--      can route requests to the correct upstream (chatgpt vs minimax).
--   2. Seed MiniMax-M2.7 as a chat model using MiniMax's
--      official OpenAI-compatible API.
--
-- Provider values:
--   ''         (empty) → legacy chatgpt.com reverse-engineering path (default)
--   'minimax'          → MiniMax official API (https://api.minimax.chat/v1)
--
-- Pricing (placeholder; adjust in admin UI):
--   MiniMax-M2.7  input  25 credits / 1M tokens
--                 output 75 credits / 1M tokens
-- ============================================================

ALTER TABLE `models`
    ADD COLUMN IF NOT EXISTS `provider` VARCHAR(32) NOT NULL DEFAULT ''
        COMMENT 'upstream provider: empty=chatgpt(default), minimax, ...'
        AFTER `upstream_model_slug`;

-- Seed MiniMax-M2.7 chat model
INSERT INTO `models`
  (`slug`, `type`, `provider`, `upstream_model_slug`,
   `input_price_per_1m`, `output_price_per_1m`,
   `cache_read_price_per_1m`, `image_price_per_call`,
   `description`, `enabled`)
VALUES
  ('MiniMax-M2.7', 'chat', 'minimax', 'MiniMax-M2.7',
   25000, 75000, 5000, 0,
   'MiniMax M2.7 large language model (official API)', 1)
ON DUPLICATE KEY UPDATE
  `provider`            = VALUES(`provider`),
  `upstream_model_slug` = VALUES(`upstream_model_slug`),
  `description`         = VALUES(`description`),
  `enabled`             = 1;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

UPDATE `models` SET `enabled` = 0 WHERE `slug` = 'MiniMax-M2.7';

-- Note: we intentionally leave the `provider` column in place on rollback
-- to avoid data loss if other rows were updated.

-- +goose StatementEnd
