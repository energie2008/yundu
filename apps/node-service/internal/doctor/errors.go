package doctor

import (
	"errors"

	"github.com/airport-panel/config"
)

var (
	ErrReportNotFound    = errors.New("doctor report not found")
	ErrCheckDefNotFound  = errors.New("check def not found")
	ErrNodeInfoMissing   = errors.New("node info required for doctor check")
	ErrAutoFixNotAvailable = errors.New("auto fix not available for this check")
)

// MapDoctorErrorToCode 将 doctor 包的错误映射到 ErrorCode
func MapDoctorErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrReportNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrCheckDefNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrNodeInfoMissing):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrAutoFixNotAvailable):
		return config.CodeBadRequest, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}
