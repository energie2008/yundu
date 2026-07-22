package importer

import (
	"errors"

	"github.com/airport-panel/config"
)

var (
	ErrImportJobNotFound  = errors.New("import job not found")
	ErrParseFailed        = errors.New("config parse failed")
	ErrUnsupportedSource  = errors.New("unsupported source type")
	ErrAlreadyApplied     = errors.New("import job already applied")
	ErrNoPreviewSpec      = errors.New("no preview node spec; parse first")
)

// MapImporterErrorToCode 将 importer 包的错误映射到 ErrorCode
func MapImporterErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrImportJobNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrParseFailed):
		return config.CodeValidationFailed, err.Error()
	case errors.Is(err, ErrUnsupportedSource):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrAlreadyApplied):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrNoPreviewSpec):
		return config.CodeBadRequest, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}
