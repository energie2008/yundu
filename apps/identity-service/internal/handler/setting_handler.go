package handler

import (
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/middleware"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type SettingHandler struct {
	settingService *service.SettingService
}

func NewSettingHandler(settingService *service.SettingService) *SettingHandler {
	return &SettingHandler{settingService: settingService}
}

func (h *SettingHandler) GetSettings(c *gin.Context) {
	group := c.Query("group")
	settings, err := h.settingService.GetSettings(c.Request.Context(), group)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, settings)
}

func (h *SettingHandler) UpdateSetting(c *gin.Context) {
	group := c.Param("group")
	key := c.Param("key")
	if group == "" || key == "" {
		server.BadRequest(c, "group and key are required")
		return
	}

	var req model.UpdateSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	adminID := middleware.GetAdminID(c)
	var updatedBy *uuid.UUID
	if adminID != uuid.Nil {
		updatedBy = &adminID
	}

	setting, err := h.settingService.UpdateSetting(c.Request.Context(), group, key, req.Value, updatedBy)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	server.OK(c, setting)
}
