package pkg

import (
	"errors"
	"time"

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
	UserID      uuid.UUID `json:"user_id"`
	TokenType   TokenType `json:"token_type"`
	SessionID   uuid.UUID `json:"session_id"`
	IsAdmin     bool      `json:"is_admin"`
	Permissions []string  `json:"permissions,omitempty"`
	AdminID     uuid.UUID `json:"admin_id,omitempty"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	secret            []byte
	accessTTLSeconds  int
	refreshTTLSeconds int
}

func NewJWTManager(secret string, accessTTLSeconds, refreshTTLSeconds int) *JWTManager {
	return &JWTManager{
		secret:            []byte(secret),
		accessTTLSeconds:  accessTTLSeconds,
		refreshTTLSeconds: refreshTTLSeconds,
	}
}

func (m *JWTManager) GenerateAccessToken(userID, sessionID, adminID uuid.UUID, isAdmin bool, permissions []string) (string, time.Time, error) {
	expiresAt := time.Now().Add(time.Duration(m.accessTTLSeconds) * time.Second)
	claims := &Claims{
		UserID:      userID,
		TokenType:   TokenTypeAccess,
		SessionID:   sessionID,
		IsAdmin:     isAdmin,
		Permissions: permissions,
		AdminID:     adminID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			ID:        uuid.New().String(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return tokenStr, expiresAt, nil
}

func (m *JWTManager) GenerateRefreshToken(userID, sessionID, adminID uuid.UUID, isAdmin bool, permissions []string) (string, string, time.Time, error) {
	refreshTokenID := uuid.New()
	expiresAt := time.Now().Add(time.Duration(m.refreshTTLSeconds) * time.Second)
	claims := &Claims{
		UserID:      userID,
		TokenType:   TokenTypeRefresh,
		SessionID:   sessionID,
		IsAdmin:     isAdmin,
		Permissions: permissions,
		AdminID:     adminID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			ID:        refreshTokenID.String(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(m.secret)
	if err != nil {
		return "", "", time.Time{}, err
	}
	return tokenStr, refreshTokenID.String(), expiresAt, nil
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

func (m *JWTManager) ValidateToken(tokenString string, expectedType TokenType) (*Claims, error) {
	claims, err := m.ParseToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != expectedType {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func (m *JWTManager) AccessTTL() time.Duration {
	return time.Duration(m.accessTTLSeconds) * time.Second
}

func (m *JWTManager) RefreshTTL() time.Duration {
	return time.Duration(m.refreshTTLSeconds) * time.Second
}
