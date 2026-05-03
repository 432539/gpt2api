// Package errcode 定义业务错误码。
// 编码规则：HTTP_STATUS + 3 位业务子码（见 docs/02-后端规范.md §3）。
package errcode

import (
	"errors"
	"fmt"
)

// Error 业务错误。
type Error struct {
	Code   int    `json:"code"`
	Msg    string `json:"msg"`
	HTTP   int    `json:"-"`
	cause  error
	detail string
}

// Error 实现 error。
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.detail != "" {
		return fmt.Sprintf("[%d] %s: %s", e.Code, e.Msg, e.detail)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Msg)
}

// Unwrap 暴露底层 error。
func (e *Error) Unwrap() error { return e.cause }

// Wrap 携带底层 error。
func (e *Error) Wrap(err error) *Error {
	if e == nil {
		return nil
	}
	ne := *e
	ne.cause = err
	if err != nil {
		ne.detail = err.Error()
	}
	return &ne
}

// WithMsg 替换 msg（注意保留 code）。
func (e *Error) WithMsg(msg string) *Error {
	if e == nil {
		return nil
	}
	ne := *e
	ne.Msg = msg
	return &ne
}

// HTTPStatus 返回建议的 HTTP 状态码。
func (e *Error) HTTPStatus() int {
	if e == nil || e.HTTP == 0 {
		return 200
	}
	return e.HTTP
}

// As 适配 errors.As。
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// New 自定义错误构造（一般业务用预定义常量；需要临时码时用本函数）。
func New(code, http int, msg string) *Error {
	return &Error{Code: code, HTTP: http, Msg: msg}
}

// === 预定义业务错误 ===
var (
	OK = &Error{Code: 0, HTTP: 200, Msg: "ok"}

	// 400xxx 参数错误
	InvalidParam = &Error{Code: 400101, HTTP: 400, Msg: "参数错误"}
	BadRequest   = &Error{Code: 400102, HTTP: 400, Msg: "请求格式不正确"}
	BodyTooLarge = &Error{Code: 400103, HTTP: 413, Msg: "请求体过大"}

	// 401xxx 鉴权
	Unauthorized = &Error{Code: 401101, HTTP: 401, Msg: "未登录"}
	TokenExpired = &Error{Code: 401102, HTTP: 401, Msg: "登录已过期"}
	TokenInvalid = &Error{Code: 401103, HTTP: 401, Msg: "登录凭证无效"}
	APIKeyInvalid = &Error{Code: 401104, HTTP: 401, Msg: "API Key 无效"}

	// 403xxx 权限
	Forbidden     = &Error{Code: 403101, HTTP: 403, Msg: "权限不足"}
	IPNotAllowed  = &Error{Code: 403102, HTTP: 403, Msg: "IP 不在白名单"}

	// 404xxx 资源不存在
	UserNotFound    = &Error{Code: 404101, HTTP: 404, Msg: "用户不存在"}
	ResourceMissing = &Error{Code: 404102, HTTP: 404, Msg: "资源不存在"}

	// 409xxx 冲突 / 幂等
	UserExists      = &Error{Code: 409101, HTTP: 409, Msg: "用户已存在"}
	IdemConflict    = &Error{Code: 409102, HTTP: 409, Msg: "重复请求"}
	DuplicatedPay   = &Error{Code: 409401, HTTP: 409, Msg: "重复支付"}

	// 429xxx 限流
	RateLimited      = &Error{Code: 429101, HTTP: 429, Msg: "操作过于频繁"}
	GenRateLimited   = &Error{Code: 429301, HTTP: 429, Msg: "创作频次超限"}

	// 500xxx 系统错误
	Internal     = &Error{Code: 500001, HTTP: 500, Msg: "系统繁忙"}
	DBError      = &Error{Code: 500002, HTTP: 500, Msg: "数据库错误"}
	CacheError   = &Error{Code: 500003, HTTP: 500, Msg: "缓存错误"}
	JobDispatch  = &Error{Code: 500301, HTTP: 500, Msg: "任务调度失败"}

	// 502xxx 上游
	GPTUnavailable  = &Error{Code: 502201, HTTP: 502, Msg: "GPT 服务暂不可用"}
	GROKUnavailable = &Error{Code: 502202, HTTP: 502, Msg: "GROK 服务暂不可用"}
	NoAvailableAcc  = &Error{Code: 502203, HTTP: 502, Msg: "暂无可用账号"}

	// 计费类
	InsufficientPoints = &Error{Code: 400401, HTTP: 400, Msg: "点数不足"}
	PromoExpired       = &Error{Code: 400402, HTTP: 400, Msg: "优惠码已失效"}
	PromoUsed          = &Error{Code: 400403, HTTP: 400, Msg: "优惠码已被使用"}
	CDKInvalid         = &Error{Code: 400404, HTTP: 400, Msg: "兑换码无效"}
	CDKUsed            = &Error{Code: 400405, HTTP: 400, Msg: "兑换码已被使用"}
)
