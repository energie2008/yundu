package exposure

import (
	"errors"

	"github.com/airport-panel/config"
)

var (
	ErrExposureNotFound      = errors.New("edge exposure not found")
	ErrExposureAlreadyExists = errors.New("edge exposure already exists")
	ErrExposureIncompatible   = errors.New("exposure mode incompatible with node protocol")
	ErrConfigRenderFailed    = errors.New("failed to render config")
	ErrNodeInfoMissing       = errors.New("node info required for render/validate")
	ErrCompatRuleDenied      = errors.New("compatibility rule denied the apply")
)

// MapExposureErrorToCode 将 exposure 包的错误映射到 ErrorCode
func MapExposureErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrExposureNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrExposureAlreadyExists):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrExposureIncompatible):
		return config.CodeValidationFailed, err.Error()
	case errors.Is(err, ErrConfigRenderFailed):
		return config.CodeInternalError, err.Error()
	case errors.Is(err, ErrNodeInfoMissing):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrCompatRuleDenied):
		return config.CodeValidationFailed, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}
