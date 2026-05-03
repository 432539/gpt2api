-- +goose Up
-- +goose StatementBegin
INSERT INTO `account_group` (`provider`, `code`, `name`, `strategy`)
VALUES ('grok', 'grok-web-default', 'Grok Web 账号池', 'round_robin')
ON DUPLICATE KEY UPDATE `name`=VALUES(`name`);

INSERT INTO `model` (`code`, `name`, `kind`, `provider`, `version`, `tags`, `point_per_unit`, `unit`, `group_code`, `min_plan`, `is_hot`, `sort`)
VALUES
('grok-4.20-fast', 'Grok 4.20 Fast', 'text', 'grok', 'chat', '文字,对话,Grok', 100, '1k_token', 'grok-web-default', 'free', 1, 10),
('grok-4.20-auto', 'Grok 4.20 Auto', 'text', 'grok', 'chat', '文字,对话,Grok', 150, '1k_token', 'grok-web-default', 'free', 1, 11),
('grok-4.20-expert', 'Grok 4.20 Expert', 'text', 'grok', 'chat', '文字,对话,Grok', 200, '1k_token', 'grok-web-default', 'free', 0, 12),
('grok-4.20-heavy', 'Grok 4.20 Heavy', 'text', 'grok', 'chat', '文字,对话,Grok', 400, '1k_token', 'grok-web-default', 'free', 0, 13),
('grok-4.3-beta', 'Grok 4.3 Beta', 'text', 'grok', 'chat', '文字,对话,Grok', 300, '1k_token', 'grok-web-default', 'free', 0, 14),
('grok-imagine-video', 'Grok Imagine Video', 'video', 'grok', 'video', '文生视频,图生视频,多图生视频,Grok', 2000, 'video', 'grok-web-default', 'free', 1, 20)
ON DUPLICATE KEY UPDATE
`name`=VALUES(`name`), `kind`=VALUES(`kind`), `provider`=VALUES(`provider`), `version`=VALUES(`version`),
`tags`=VALUES(`tags`), `point_per_unit`=VALUES(`point_per_unit`), `unit`=VALUES(`unit`), `group_code`=VALUES(`group_code`);

INSERT INTO `system_config` (`key`, `value`, `remark`)
SELECT 'billing.model_prices',
       '[]',
       '模型价格、上游映射和文字 token 计费'
WHERE NOT EXISTS (SELECT 1 FROM `system_config` WHERE `key`='billing.model_prices');

UPDATE `system_config`
SET `value` = JSON_ARRAY_APPEND(CAST(`value` AS JSON), '$', CAST('{"model_code":"grok-4.20-fast","name":"Grok 4.20 Fast","kind":"text","provider":"grok","upstream_model":"grok-4.20-fast","unit_points":0,"input_unit_points":100,"output_unit_points":300,"enabled":true}' AS JSON))
WHERE `key`='billing.model_prices' AND JSON_SEARCH(CAST(`value` AS JSON), 'one', 'grok-4.20-fast', NULL, '$[*].model_code') IS NULL;

UPDATE `system_config`
SET `value` = JSON_ARRAY_APPEND(CAST(`value` AS JSON), '$', CAST('{"model_code":"grok-4.20-auto","name":"Grok 4.20 Auto","kind":"text","provider":"grok","upstream_model":"grok-4.20-auto","unit_points":0,"input_unit_points":150,"output_unit_points":450,"enabled":true}' AS JSON))
WHERE `key`='billing.model_prices' AND JSON_SEARCH(CAST(`value` AS JSON), 'one', 'grok-4.20-auto', NULL, '$[*].model_code') IS NULL;

UPDATE `system_config`
SET `value` = JSON_ARRAY_APPEND(CAST(`value` AS JSON), '$', CAST('{"model_code":"grok-4.20-expert","name":"Grok 4.20 Expert","kind":"text","provider":"grok","upstream_model":"grok-4.20-expert","unit_points":0,"input_unit_points":200,"output_unit_points":600,"enabled":true}' AS JSON))
WHERE `key`='billing.model_prices' AND JSON_SEARCH(CAST(`value` AS JSON), 'one', 'grok-4.20-expert', NULL, '$[*].model_code') IS NULL;

UPDATE `system_config`
SET `value` = JSON_ARRAY_APPEND(CAST(`value` AS JSON), '$', CAST('{"model_code":"grok-4.20-heavy","name":"Grok 4.20 Heavy","kind":"text","provider":"grok","upstream_model":"grok-4.20-heavy","unit_points":0,"input_unit_points":400,"output_unit_points":1200,"enabled":true}' AS JSON))
WHERE `key`='billing.model_prices' AND JSON_SEARCH(CAST(`value` AS JSON), 'one', 'grok-4.20-heavy', NULL, '$[*].model_code') IS NULL;

UPDATE `system_config`
SET `value` = JSON_ARRAY_APPEND(CAST(`value` AS JSON), '$', CAST('{"model_code":"grok-4.3-beta","name":"Grok 4.3 Beta","kind":"text","provider":"grok","upstream_model":"grok-4.3-beta","unit_points":0,"input_unit_points":300,"output_unit_points":900,"enabled":true}' AS JSON))
WHERE `key`='billing.model_prices' AND JSON_SEARCH(CAST(`value` AS JSON), 'one', 'grok-4.3-beta', NULL, '$[*].model_code') IS NULL;

UPDATE `system_config`
SET `value` = JSON_ARRAY_APPEND(CAST(`value` AS JSON), '$', CAST('{"model_code":"grok-imagine-video","name":"Grok Imagine Video","kind":"video","provider":"grok","upstream_model":"grok-imagine-video","unit_points":2000,"enabled":true}' AS JSON))
WHERE `key`='billing.model_prices' AND JSON_SEARCH(CAST(`value` AS JSON), 'one', 'grok-imagine-video', NULL, '$[*].model_code') IS NULL;
-- +goose StatementEnd

-- +goose Down
DELETE FROM `model` WHERE `code` IN ('grok-4.20-fast','grok-4.20-auto','grok-4.20-expert','grok-4.20-heavy','grok-4.3-beta','grok-imagine-video');
