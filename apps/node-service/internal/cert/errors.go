package cert

import (
	"errors"

	"github.com/airport-panel/config"
)

var (
	ErrCertNotFound       = errors.New("certificate not found")
	ErrCertAlreadyExists  = errors.New("certificate already exists")
	ErrProfileNotFound    = errors.New("tls profile not found")
	ErrProfileAlreadyExists = errors.New("tls profile already exists")
	ErrProfileInUse       = errors.New("tls profile in use by edge exposures")
	ErrCertInvalid        = errors.New("invalid certificate payload")
	ErrRenewNotApplicable = errors.New("renew not applicable for non-auto cert")
	ErrDeployRecordNotFound = errors.New("deploy record not found")
	// ErrNodeSNIReaderNotInjected 当未注入 NodeSNIReader 时 SyncSANFromNodes 返回此错误。
	ErrNodeSNIReaderNotInjected = errors.New("node SNI reader not injected")
)

// MapCertErrorToCode 将 cert 包的错误映射到 ErrorCode
func MapCertErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrCertNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrCertAlreadyExists):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrProfileNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrProfileAlreadyExists):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrProfileInUse):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrCertInvalid):
		return config.CodeValidationFailed, err.Error()
	case errors.Is(err, ErrRenewNotApplicable):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrDeployRecordNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrNodeSNIReaderNotInjected):
		return config.CodeBadRequest, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}
