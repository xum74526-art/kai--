package repository

import (
	"context"
	"errors"
)

// Ping 复用现有连接池检查数据库是否可用。就绪探针不能绕过 repository
// 触达 GORM，也不能为了探测再建立第二套连接池。
func (r *Repository) Ping(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("database repository is not initialized")
	}
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	return db.PingContext(ctx)
}
