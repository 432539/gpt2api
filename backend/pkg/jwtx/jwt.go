// Package jwtx 双 Token 实现：Access（短）+ Refresh（长）。
// HS256 + jti 单设备绑定。
package jwtx

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Subject 主体类型，区分用户端 / 后台端。
type Subject string

const (
	SubjectUser  Subject = "user"
	SubjectAdmin Subject = "admin"
)

// Claims 自定义负载。
type Claims struct {
	UID     uint64   `json:"uid"`
	Subject Subject  `json:"sub_t"`
	Roles   []string `json:"roles,omitempty"`
	Scope   string   `json:"scope,omitempty"`
	JTI     string   `json:"jti"`
	jwt.RegisteredClaims
}

// Manager 颁发与校验 token。
type Manager struct {
	secret        []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
	issuer        string
}

// New 创建 Manager。
func New(secret, refreshSecret string, accessTTL, refreshTTL time.Duration) (*Manager, error) {
	if len(secret) < 16 || len(refreshSecret) < 16 {
		return nil, errors.New("jwt secret must >= 16 bytes")
	}
	return &Manager{
		secret:        []byte(secret),
		refreshSecret: []byte(refreshSecret),
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
		issuer:        "kleinai",
	}, nil
}

// IssueAccess 签发 Access Token。
func (m *Manager) IssueAccess(uid uint64, sub Subject, jti string, roles []string) (string, time.Time, error) {
	exp := time.Now().Add(m.accessTTL)
	c := Claims{
		UID:     uid,
		Subject: sub,
		Roles:   roles,
		JTI:     jti,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   fmt.Sprintf("%d", uid),
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(m.secret)
	return tok, exp, err
}

// IssueRefresh 签发 Refresh Token。
func (m *Manager) IssueRefresh(uid uint64, sub Subject, jti string) (string, time.Time, error) {
	exp := time.Now().Add(m.refreshTTL)
	c := Claims{
		UID:     uid,
		Subject: sub,
		JTI:     jti,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   fmt.Sprintf("%d", uid),
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(m.refreshSecret)
	return tok, exp, err
}

// ParseAccess 解析并校验 Access。
func (m *Manager) ParseAccess(tok string) (*Claims, error) { return m.parse(tok, m.secret) }

// ParseRefresh 解析并校验 Refresh。
func (m *Manager) ParseRefresh(tok string) (*Claims, error) { return m.parse(tok, m.refreshSecret) }

func (m *Manager) parse(tok string, key []byte) (*Claims, error) {
	c := &Claims{}
	parsed, err := jwt.ParseWithClaims(tok, c, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Method)
		}
		return key, nil
	})
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return c, nil
}
