package repo

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kleinai/backend/internal/model"
)

// SystemConfigRepo system_config 仓储。
type SystemConfigRepo struct{ db *gorm.DB }

// NewSystemConfigRepo 构造。
func NewSystemConfigRepo(db *gorm.DB) *SystemConfigRepo { return &SystemConfigRepo{db: db} }

// GetAll 全表。
func (r *SystemConfigRepo) GetAll(ctx context.Context) ([]*model.SystemConfig, error) {
	var rows []*model.SystemConfig
	err := r.db.WithContext(ctx).Find(&rows).Error
	return rows, err
}

// GetByKey 单 key。
func (r *SystemConfigRepo) GetByKey(ctx context.Context, key string) (*model.SystemConfig, error) {
	var row model.SystemConfig
	err := r.db.WithContext(ctx).Where("`key` = ?", key).First(&row).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &row, nil
}

// GetMany 批量获取，未存在的 key 返回空。
func (r *SystemConfigRepo) GetMany(ctx context.Context, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}
	var rows []*model.SystemConfig
	if err := r.db.WithContext(ctx).Where("`key` IN ?", keys).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Key] = r.Value
	}
	return out, nil
}

// Upsert 插入或更新。
func (r *SystemConfigRepo) Upsert(ctx context.Context, key, value string, updatedBy *uint64, remark *string) error {
	row := &model.SystemConfig{Key: key, Value: value, UpdatedBy: updatedBy, Remark: remark}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "updated_by", "updated_at"}),
		}).
		Create(row).Error
}

// UpsertMany 批量 upsert（同事务）。
func (r *SystemConfigRepo) UpsertMany(ctx context.Context, kvs map[string]string, updatedBy *uint64) error {
	if len(kvs) == 0 {
		return nil
	}
	rows := make([]*model.SystemConfig, 0, len(kvs))
	for k, v := range kvs {
		rows = append(rows, &model.SystemConfig{Key: k, Value: v, UpdatedBy: updatedBy})
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "updated_by", "updated_at"}),
		}).
		Create(&rows).Error
}
