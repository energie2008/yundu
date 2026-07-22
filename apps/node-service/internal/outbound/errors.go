package outbound

import (
	"errors"

	"github.com/airport-panel/config"
)

var (
	ErrPolicyNotFound       = errors.New("outbound policy not found")
	ErrPolicyAlreadyExists  = errors.New("outbound policy already exists for this node")
	ErrWarpProfileNotFound  = errors.New("warp profile not found")
	ErrWarpProfileExists     = errors.New("warp profile already exists")
	ErrInvalidPolicyConfig  = errors.New("invalid outbound policy config")
	ErrRenderFailed         = errors.New("failed to render outbounds")
)

// MapOutboundErrorToCode 将 outbound 包的错误映射到 ErrorCode
func MapOutboundErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrPolicyNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrPolicyAlreadyExists):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrWarpProfileNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrWarpProfileExists):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrInvalidPolicyConfig):
		return config.CodeValidationFailed, err.Error()
	case errors.Is(err, ErrRenderFailed):
		return config.CodeInternalError, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}
