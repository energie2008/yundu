package upgrade

import (
	"errors"

	"github.com/airport-panel/config"
)

var (
	ErrUpgradeTaskNotFound    = errors.New("upgrade task not found")
	ErrUpgradeAlreadyRunning  = errors.New("an upgrade task is already running for this server")
	ErrVersionSame            = errors.New("from and to versions are the same")
	ErrRuntimeNotFound        = errors.New("runtime not found")
	ErrNoCanaryTargets        = errors.New("no servers selected for canary upgrade")
	ErrInvalidCanaryPercent   = errors.New("canary percent must be between 1 and 100")
	ErrTaskNotRollbackable    = errors.New("upgrade task is not in a rollbackable state")
)

// MapUpgradeErrorToCode 将 upgrade 包的错误映射到 ErrorCode
func MapUpgradeErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrUpgradeTaskNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrRuntimeNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrUpgradeAlreadyRunning):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrVersionSame):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrNoCanaryTargets):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrInvalidCanaryPercent):
		return config.CodeValidationFailed, err.Error()
	case errors.Is(err, ErrTaskNotRollbackable):
		return config.CodeConflict, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}
