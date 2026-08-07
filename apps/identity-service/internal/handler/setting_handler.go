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
	// 按 setting_group → setting_key → value 分组返回，匹配前端期望的 {group: {key: value}} 结构
	grouped := make(map[string]map[string]interface{})
	for _, st := range settings {
		if _, ok := grouped[st.SettingGroup]; !ok {
			grouped[st.SettingGroup] = make(map[string]interface{})
		}
		grouped[st.SettingGroup][st.SettingKey] = st.Value
	}
	server.OK(c, grouped)
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

// GetPublicConfig 返回 user-web 公开可见的配置子集（无需鉴权）。
// 仅返回 subscribe_domain / subscribe_path / app_name / frontend_url / app_description，
// 绝不泄露敏感设置（subscribe_key、smtp_*、支付密钥等）。
// 用途：user-web 启动时 GET /api/v1/guest/config 动态获取订阅域名，
// 避免在 endpoints.ts 硬编码 SUB_BASE，使 admin 改 subscribe_domain 后 user-web 自动跟随。
func (h *SettingHandler) GetPublicConfig(c *gin.Context) {
	ctx := c.Request.Context()
	out := map[string]interface{}{}

	// general 组：app_name / frontend_url / app_description
	if general, err := h.settingService.GetSettings(ctx, "general"); err == nil {
		for _, s := range general {
			switch s.SettingKey {
			case "app_name", "frontend_url", "app_description":
				out[s.SettingKey] = s.Value
			}
		}
	}

	// subscribe 组：仅 subscribe_domain / subscribe_path（白名单，绝不返回 subscribe_key）
	if sub, err := h.settingService.GetSettings(ctx, "subscribe"); err == nil {
		for _, s := range sub {
			switch s.SettingKey {
			case "subscribe_domain", "subscribe_path":
				out[s.SettingKey] = s.Value
			}
		}
	}

	server.OK(c, out)
}
