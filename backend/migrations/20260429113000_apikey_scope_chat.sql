-- +goose Up
-- +goose StatementBegin
ALTER TABLE `api_key`
  MODIFY COLUMN `scope` VARCHAR(255) NOT NULL DEFAULT 'chat,image,video';

UPDATE `api_key`
SET `scope` = 'chat,image,video'
WHERE `scope` = 'image,video';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE `api_key`
SET `scope` = 'image,video'
WHERE `scope` = 'chat,image,video';

ALTER TABLE `api_key`
  MODIFY COLUMN `scope` VARCHAR(255) NOT NULL DEFAULT 'image,video';
-- +goose StatementEnd
