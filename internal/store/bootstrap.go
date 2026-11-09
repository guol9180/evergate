package store

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// 默认管理员（任务书 4.3：首次启动若空表创建 admin/admin123 并强制首登改密）。
const (
	DefaultAdminUsername = "admin"
	DefaultAdminPassword = "admin123"
)

// EnsureAdmin 在 admins 空表时创建默认管理员，返回是否创建。
func EnsureAdmin(db *gorm.DB, username, password string) (bool, error) {
	var count int64
	if err := db.Model(&Admin{}).Count(&count).Error; err != nil {
		return false, fmt.Errorf("count admins: %w", err)
	}
	if count > 0 {
		return false, nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, fmt.Errorf("hash admin password: %w", err)
	}
	admin := Admin{Username: username, PasswordHash: string(hash), MustChangePassword: true}
	if err := db.Create(&admin).Error; err != nil {
		return false, fmt.Errorf("create default admin: %w", err)
	}
	return true, nil
}

// GetAdminByUsername 按用户名查询管理员（登录用，T5 接线）。
func GetAdminByUsername(db *gorm.DB, username string) (*Admin, error) {
	var admin Admin
	if err := db.Where("username = ?", username).First(&admin).Error; err != nil {
		return nil, err
	}
	return &admin, nil
}

// CheckAdminPassword 校验管理员密码（bcrypt 比对）。
func CheckAdminPassword(admin *Admin, password string) bool {
	return admin != nil && bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)) == nil
}
