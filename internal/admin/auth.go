package admin

// 会话与账号：登录 / 首启 / 登出 / 当前用户 / 改密，以及只允许本机访问与会话校验的中间件。

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"evergate/internal/httpx"
	"evergate/internal/store"
)

func (a *API) accessMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.opt.AllowRemote || isLoopback(c.ClientIP()) {
			c.Next()
			return
		}
		fail(c, http.StatusForbidden, "admin_local_only", "管理 API 仅允许本机访问；远程管理请显式关闭 admin_loopback_only")
		c.Abort()
	}
}

func (a *API) login(c *gin.Context) {
	clientIP := c.ClientIP()
	if wait := a.loginLimiter.blockedFor(clientIP, a.now()); wait > 0 {
		seconds := (wait + time.Second - 1) / time.Second
		c.Header("Retry-After", strconv.FormatInt(int64(seconds), 10))
		fail(c, http.StatusTooManyRequests, "login_rate_limited", "登录尝试过多，请稍后重试")
		return
	}
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "credentials_required", "请输入用户名和密码")
		return
	}
	admin, err := store.GetAdminByUsername(a.opt.DB, req.Username)
	if err != nil || !store.CheckAdminPassword(admin, req.Password) {
		a.loginLimiter.recordFailure(clientIP, a.now())
		fail(c, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
		return
	}
	a.loginLimiter.recordSuccess(clientIP)
	now := a.now()
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(CookieName, a.opt.Session.Issue(req.Username, now), int(a.opt.Session.ttl.Seconds()), "/", "", c.Request.TLS != nil, true)
	ok(c, gin.H{"username": admin.Username, "must_change_password": admin.MustChangePassword})
}

// setupStatus 供登录页判断是否需要先完成首启管理员设置。
func (a *API) setupStatus(c *gin.Context) {
	required, err := store.SetupRequired(a.opt.DB)
	if err != nil {
		fail(c, http.StatusInternalServerError, "setup_status_failed", "查询初始化状态失败")
		return
	}
	ok(c, gin.H{"setup_required": required})
}

// setupAdmin 首启管理员设置：仅在 admins 为空且请求来自本机时允许；成功后不自动登录。
func (a *API) setupAdmin(c *gin.Context) {
	if !isLoopback(c.ClientIP()) {
		fail(c, http.StatusForbidden, "setup_local_only", "初始管理员设置仅允许本机访问")
		return
	}
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误")
		return
	}
	username := strings.TrimSpace(req.Username)
	if !adminUsernamePattern.MatchString(username) {
		fail(c, http.StatusBadRequest, "invalid_username", "用户名需为 3–32 位字母、数字、下划线、点或连字符")
		return
	}
	if err := store.ValidateAdminPassword(req.Password); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", err.Error())
		return
	}
	// 单进程内串行，保证并发设置只有一个成功；跨进程由用户名唯一索引兜底
	a.setupMu.Lock()
	defer a.setupMu.Unlock()
	required, err := store.SetupRequired(a.opt.DB)
	if err != nil {
		fail(c, http.StatusInternalServerError, "setup_status_failed", "查询初始化状态失败")
		return
	}
	if !required {
		fail(c, http.StatusForbidden, "already_initialized", "管理员已存在，无法重复初始化")
		return
	}
	if err := store.CreateAdmin(a.opt.DB, username, req.Password); err != nil {
		if stillRequired, _ := store.SetupRequired(a.opt.DB); !stillRequired {
			fail(c, http.StatusForbidden, "already_initialized", "管理员已存在，无法重复初始化")
			return
		}
		fail(c, http.StatusInternalServerError, "setup_failed", "创建管理员失败")
		return
	}
	c.Set("admin", username)
	a.recordAudit(c, "setup_admin", "", "username="+username)
	ok(c, gin.H{"username": username, "setup": true})
}

func (a *API) logout(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(CookieName, "", -1, "/", "", false, true)
	ok(c, gin.H{"logout": true})
}

func (a *API) me(c *gin.Context) {
	admin, err := store.GetAdminByUsername(a.opt.DB, c.GetString("admin"))
	if err != nil {
		fail(c, http.StatusUnauthorized, "session_invalid", "会话无效")
		return
	}
	ok(c, gin.H{"username": admin.Username, "must_change_password": admin.MustChangePassword})
}

func (a *API) changePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "password_too_short", "参数错误（新密码至少 8 位）")
		return
	}
	admin, err := store.GetAdminByUsername(a.opt.DB, c.GetString("admin"))
	if err != nil || !store.CheckAdminPassword(admin, req.OldPassword) {
		fail(c, http.StatusUnauthorized, "wrong_old_password", "旧密码错误")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		fail(c, http.StatusInternalServerError, "hash_failed", "加密失败")
		return
	}
	if err := a.opt.DB.Model(&store.Admin{}).Where("id = ?", admin.ID).
		Updates(map[string]any{"password_hash": string(hash), "must_change_password": false}).Error; err != nil {
		fail(c, http.StatusInternalServerError, "update_failed", "更新失败")
		return
	}
	ok(c, gin.H{"changed": true})
}

// sessionMiddleware 校验会话 Cookie；首登未改密前仅放行 me/password（强制改密）。
// 请求结束后对白名单内的写操作落审计（T75）。
func (a *API) sessionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.authenticateSession(c) {
			return
		}
		c.Next()
		a.auditWrite(c)
	}
}

// authenticateSession 校验会话并在需要时中止请求；返回 false 表示已中止。
func (a *API) authenticateSession(c *gin.Context) bool {
	token, err := c.Cookie(CookieName)
	if err != nil || token == "" {
		fail(c, http.StatusUnauthorized, "unauthorized", "未登录")
		c.Abort()
		return false
	}
	username, verr := a.opt.Session.Verify(token, a.now())
	if verr != nil {
		fail(c, http.StatusUnauthorized, "session_expired", "会话无效或已过期")
		c.Abort()
		return false
	}
	c.Set("admin", username)
	// 强制首登改密：me/password 放行，其余一律 403
	path := c.FullPath()
	if strings.HasSuffix(path, "/auth/me") || strings.HasSuffix(path, "/auth/password") {
		return true
	}
	admin, aerr := store.GetAdminByUsername(a.opt.DB, username)
	if aerr == nil && admin.MustChangePassword {
		httpx.FailCode(c, http.StatusForbidden, "must_change_password", "首次登录必须修改密码")
		c.Abort()
		return false
	}
	return true
}

// ---------- 渠道 ----------
