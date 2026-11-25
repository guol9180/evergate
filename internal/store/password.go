package store

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"unicode"
)

// 随机密码字符集：四类字符各至少一个，保证常见密码策略均可通过。
const (
	pwLower   = "abcdefghijklmnopqrstuvwxyz"
	pwUpper   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	pwDigits  = "0123456789"
	pwSymbols = "!@#$%^&*-_+="
)

// InitialAdminPassword 决定首次启动管理员密码：环境变量 EVERGATE_INITIAL_ADMIN_PASSWORD 非空时优先（自动化/容器场景），
// 否则生成 16 位随机密码。返回是否来自环境变量。
func InitialAdminPassword(envValue string) (string, bool) {
	if v := strings.TrimSpace(envValue); v != "" {
		return v, true
	}
	return GenerateRandomPassword(16), false
}

// GenerateRandomPassword 生成长度 n 的密码，保证同时含大写、小写、数字、符号（n ≥ 4），其余位从全字符集随机填充后洗牌。
func GenerateRandomPassword(n int) string {
	if n < 4 {
		n = 4
	}
	classes := []string{pwLower, pwUpper, pwDigits, pwSymbols}
	chars := make([]byte, 0, n)
	for _, class := range classes {
		chars = append(chars, class[randomIndex(len(class))])
	}
	all := pwLower + pwUpper + pwDigits + pwSymbols
	for len(chars) < n {
		chars = append(chars, all[randomIndex(len(all))])
	}
	// Fisher–Yates 洗牌，避免"前四位固定为四类"的可预测布局。
	for i := len(chars) - 1; i > 0; i-- {
		j := randomIndex(i + 1)
		chars[i], chars[j] = chars[j], chars[i]
	}
	return string(chars)
}

func randomIndex(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		// crypto/rand 在主流平台不会失败；若失败直接 panic 好于返回可预测密码。
		panic("evergate: crypto/rand unavailable: " + err.Error())
	}
	return int(v.Int64())
}

// ValidateAdminPassword 首启设置向导的密码策略：至少 8 位，且包含大写 / 小写 / 数字 / 符号中的至少三类。
func ValidateAdminPassword(password string) error {
	if len([]rune(password)) < 8 {
		return errors.New("密码至少 8 位")
	}
	var lower, upper, digit, symbol bool
	for _, r := range password {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		default:
			symbol = true
		}
	}
	classes := 0
	for _, ok := range []bool{lower, upper, digit, symbol} {
		if ok {
			classes++
		}
	}
	if classes < 3 {
		return errors.New("密码需至少包含三类字符（大写、小写、数字、符号）")
	}
	return nil
}
