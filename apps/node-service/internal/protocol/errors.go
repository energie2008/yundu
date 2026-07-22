package protocol

import (
	"errors"

	"github.com/airport-panel/config"
)

var (
	ErrProtocolNotFound    = errors.New("protocol registry not found")
	ErrProtocolExists       = errors.New("protocol registry already exists for this combination")
	ErrTemplateNotFound    = errors.New("config template not found")
	ErrSchemaValidation    = errors.New("config schema validation failed")
	ErrTemplateRender       = errors.New("failed to render config template")
	ErrInvalidSchemaPayload = errors.New("invalid config schema payload")
)

// MapProtocolErrorToCode 将 protocol 包的错误映射到 ErrorCode
func MapProtocolErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrProtocolNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrProtocolExists):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrTemplateNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrSchemaValidation):
		return config.CodeValidationFailed, err.Error()
	case errors.Is(err, ErrTemplateRender):
		return config.CodeInternalError, err.Error()
	case errors.Is(err, ErrInvalidSchemaPayload):
		return config.CodeValidationFailed, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}
