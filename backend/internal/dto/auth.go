// Package dto 接口出入参，仅做形状定义与校验，不含业务逻辑。
package dto

// RegisterReq 注册请求。
type RegisterReq struct {
	Account    string `json:"account"     binding:"required,min=3,max=64"`     // 邮箱 / 手机 / 用户名
	Password   string `json:"password"    binding:"required,min=8,max=64"`
	Code       string `json:"code"        binding:"omitempty,len=6"`           // 短信 / 邮箱验证码
	InviteCode string `json:"invite_code" binding:"omitempty,max=16"`
}

// LoginReq 登录请求。
type LoginReq struct {
	Account  string `json:"account"  binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=64"`
}

// RefreshReq 刷新请求。
type RefreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// ChangePasswordReq 改密请求。
type ChangePasswordReq struct {
	OldPassword string `json:"old_password" binding:"required,min=6,max=64"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=64"`
}

// TokenPair 颁发的 token 对。
type TokenPair struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	AccessExpireIn   int64  `json:"access_expire_in"`
	RefreshExpireIn  int64  `json:"refresh_expire_in"`
}

// MeResp 当前用户信息。
type MeResp struct {
	UID         uint64  `json:"uid"`
	UUID        string  `json:"uuid"`
	Username    *string `json:"username,omitempty"`
	Email       *string `json:"email,omitempty"`
	Phone       *string `json:"phone,omitempty"`
	Avatar      *string `json:"avatar,omitempty"`
	Points      int64   `json:"points"`
	FrozenPts   int64   `json:"frozen_points"`
	PlanCode    string  `json:"plan_code"`
	InviteCode  string  `json:"invite_code"`
	CreatedAt   int64   `json:"created_at"`
}
