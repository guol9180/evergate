package store

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// DefaultAdminUsername 是默认管理员用户名（任务书 4.3：首次启动若空表创建 admin 并强制首登改密）。
// 初始密码不再固定：由 InitialAdminPassword 随机生成或读取 EVERGATE_INITIAL_ADMIN_PASSWORD（T75）。
const DefaultAdminUsername = "admin"

// InitialAdminEnv 指定首次启动管理员密码的环境变量（自动化/容器场景）。
const InitialAdminEnv = "EVERGATE_INITIAL_ADMIN_PASSWORD"

// EnsureAdmin 在 admins 空表时创建管理员，返回是否创建。
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

// SetupRequired 报告 admins 表是否为空（首启需要用户在管理台完成管理员设置）。
func SetupRequired(db *gorm.DB) (bool, error) {
	var count int64
	if err := db.Model(&Admin{}).Count(&count).Error; err != nil {
		return false, fmt.Errorf("count admins: %w", err)
	}
	return count == 0, nil
}

// CreateAdmin 创建已设定密码的管理员，不强制首登改密（首启设置向导用）。
func CreateAdmin(db *gorm.DB, username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	admin := Admin{Username: username, PasswordHash: string(hash), MustChangePassword: false}
	// MustChangePassword 的列默认值是 true，GORM 会跳过零值插入，必须显式回写 false
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&admin).Error; err != nil {
			return fmt.Errorf("create admin: %w", err)
		}
		if err := tx.Model(&admin).Update("must_change_password", false).Error; err != nil {
			return fmt.Errorf("clear must_change_password: %w", err)
		}
		return nil
	})
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
