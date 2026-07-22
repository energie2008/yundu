package config

type ErrorCode int

const (
	CodeSuccess           ErrorCode = 0
	CodeBadRequest        ErrorCode = 40000
	CodeUnauthorized      ErrorCode = 40100
	CodeForbidden         ErrorCode = 40300
	CodeNotFound          ErrorCode = 40400
	CodeConflict          ErrorCode = 40900
	CodeValidationFailed  ErrorCode = 42200
	CodeInternalError     ErrorCode = 50000
	CodeServiceUnavailable ErrorCode = 50300
)

var errorMessages = map[ErrorCode]string{
	CodeSuccess:            "ok",
	CodeBadRequest:         "bad request",
	CodeUnauthorized:       "unauthorized",
	CodeForbidden:          "forbidden",
	CodeNotFound:           "resource not found",
	CodeConflict:           "resource conflict",
	CodeValidationFailed:   "validation failed",
	CodeInternalError:      "internal server error",
	CodeServiceUnavailable: "service unavailable",
}

func (c ErrorCode) Message() string {
	if msg, ok := errorMessages[c]; ok {
		return msg
	}
	return "unknown error"
}

func (c ErrorCode) HTTPStatus() int {
	switch {
	case c == CodeSuccess:
		return 200
	case c >= 40000 && c < 40100:
		return 400
	case c >= 40100 && c < 40300:
		return 401
	case c >= 40300 && c < 40400:
		return 403
	case c >= 40400 && c < 40900:
		return 404
	case c >= 40900 && c < 42200:
		return 409
	case c >= 42200 && c < 50000:
		return 422
	case c >= 50300:
		return 503
	default:
		return 500
	}
}
