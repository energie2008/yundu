package compat

import (
	"errors"

	"github.com/airport-panel/config"
)

// 兼容模块错误定义
var (
	ErrClientNotFound   = errors.New("client profile not found")
	ErrFeatureNotFound  = errors.New("feature not found in compat matrix")
	ErrInvalidVersion   = errors.New("invalid version format")
	ErrMatrixEntryInvalid = errors.New("compat matrix entry invalid")
	ErrPatchNotFound    = errors.New("advanced patch profile not found")
	ErrCompatSourceNotConfigured = errors.New("compat source not configured")
)

// MapCompatErrorToCode 将兼容模块错误映射为统一错误码
func MapCompatErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrClientNotFound):
		return config.CodeNotFound, "client profile not found"
	case errors.Is(err, ErrFeatureNotFound):
		return config.CodeNotFound, "feature not found in compat matrix"
	case errors.Is(err, ErrInvalidVersion):
		return config.CodeBadRequest, "invalid version format"
	case errors.Is(err, ErrMatrixEntryInvalid):
		return config.CodeBadRequest, "compat matrix entry invalid"
	case errors.Is(err, ErrPatchNotFound):
		return config.CodeNotFound, "advanced patch profile not found"
	case errors.Is(err, ErrCompatSourceNotConfigured):
		return config.CodeBadRequest, "compat source not configured"
	default:
		return config.CodeInternalError, "internal server error"
	}
}
