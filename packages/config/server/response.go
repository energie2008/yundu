package server

import (
	"github.com/airport-panel/config"
	"github.com/airport-panel/config/middleware"
	"github.com/gin-gonic/gin"
	"net/http"
)

func OK(c *gin.Context, data interface{}) {
	rid := middleware.GetRequestID(c)
	c.JSON(http.StatusOK, config.OK(data, rid))
}

func Created(c *gin.Context, data interface{}) {
	rid := middleware.GetRequestID(c)
	c.JSON(http.StatusCreated, config.OK(data, rid))
}

func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func Fail(c *gin.Context, code config.ErrorCode, message string) {
	rid := middleware.GetRequestID(c)
	c.JSON(code.HTTPStatus(), config.Fail(code, message, rid))
}

func BadRequest(c *gin.Context, message string) {
	Fail(c, config.CodeBadRequest, message)
}

func Unauthorized(c *gin.Context, message string) {
	if message == "" {
		message = "unauthorized"
	}
	Fail(c, config.CodeUnauthorized, message)
}

func Forbidden(c *gin.Context, message string) {
	if message == "" {
		message = "forbidden"
	}
	Fail(c, config.CodeForbidden, message)
}

func NotFound(c *gin.Context, message string) {
	if message == "" {
		message = "resource not found"
	}
	Fail(c, config.CodeNotFound, message)
}

func InternalError(c *gin.Context, message string) {
	if message == "" {
		message = "internal server error"
	}
	Fail(c, config.CodeInternalError, message)
}

func ValidationError(c *gin.Context, message string) {
	Fail(c, config.CodeValidationFailed, message)
}
