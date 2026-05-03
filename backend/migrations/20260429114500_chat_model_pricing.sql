-- +goose Up
-- +goose StatementBegin
INSERT INTO `account_group` (`provider`, `code`, `name`, `strategy`)
VALUES ('gpt', 'gpt-chat-default', 'GPT 通用文字', 'round_robin')
ON DUPLICATE KEY UPDATE `name`=VALUES(`name`);

INSERT INTO `model` (`code`, `name`, `kind`, `provider`, `version`, `tags`, `point_per_unit`, `unit`, `group_code`, `min_plan`, `is_hot`, `sort`)
VALUES ('gpt-4o-mini', '文字对话', 'text', 'gpt', 'chat', '文字,对话,兼容OpenAI', 100, '1k_token', 'gpt-chat-default', 'free', 1, 0)
ON DUPLICATE KEY UPDATE `name`=VALUES(`name`), `kind`=VALUES(`kind`), `provider`=VALUES(`provider`);

INSERT INTO `system_config` (`key`, `value`, `remark`)
SELECT 'billing.model_prices',
       '[{"model_code":"gpt-4o-mini","name":"文字对话","kind":"text","provider":"gpt","upstream_model":"gpt-4o-mini","unit_points":0,"input_unit_points":100,"output_unit_points":300,"enabled":true},{"model_code":"img-v3","name":"通用图片","kind":"image","provider":"gpt","upstream_model":"gpt-image","unit_points":400,"enabled":true},{"model_code":"img-real","name":"真实图片","kind":"image","provider":"gpt","upstream_model":"gpt-image-real","unit_points":400,"enabled":true},{"model_code":"img-anime","name":"动漫图片","kind":"image","provider":"gpt","upstream_model":"gpt-image-anime","unit_points":300,"enabled":true},{"model_code":"img-3d","name":"3D 图片","kind":"image","provider":"gpt","upstream_model":"gpt-image-3d","unit_points":500,"enabled":true},{"model_code":"vid-v1","name":"视频生成","kind":"video","provider":"grok","upstream_model":"grok-video","unit_points":1500,"enabled":true},{"model_code":"vid-i2v","name":"图生视频","kind":"video","provider":"grok","upstream_model":"grok-i2v","unit_points":2000,"enabled":true}]',
       '模型价格、上游映射和文字 token 计费'
WHERE NOT EXISTS (SELECT 1 FROM `system_config` WHERE `key`='billing.model_prices');
-- +goose StatementEnd

-- +goose Down
DELETE FROM `model` WHERE `code`='gpt-4o-mini';
DELETE FROM `account_group` WHERE `code`='gpt-chat-default';
