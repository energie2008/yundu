package pkg

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

type Claims struct {
	UserID      uuid.UUID   `json:"user_id"`
	TokenType   TokenType   `json:"token_type"`
	SessionID   uuid.UUID   `json:"session_id"`
	IsAdmin     bool        `json:"is_admin"`
	Permissions []string    `json:"permissions,omitempty"`
	AdminID     uuid.UUID   `json:"admin_id,omitempty"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	secret []byte
}

func NewJWTManager(secret string) *JWTManager {
	return &JWTManager{
		secret: []byte(secret),
	}
}

func (m *JWTManager) ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func (m *JWTManager) ValidateAdminToken(tokenString string) (*Claims, error) {
	claims, err := m.ParseToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != TokenTypeAccess {
		return nil, ErrInvalidToken
	}
	if !claims.IsAdmin {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
