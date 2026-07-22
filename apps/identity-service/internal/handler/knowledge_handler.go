package handler

import (
	"fmt"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/middleware"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
)

// KnowledgeHandler 知识库管理（管理端 + 用户端）
type KnowledgeHandler struct {
	knowledgeSvc *service.KnowledgeService
}

func NewKnowledgeHandler(knowledgeSvc *service.KnowledgeService) *KnowledgeHandler {
	return &KnowledgeHandler{knowledgeSvc: knowledgeSvc}
}

func (h *KnowledgeHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	g := admin.Group("/knowledge")
	{
		g.GET("/getCategory", rbac.RequirePermission("knowledge.read"), h.ListCategories)
		g.GET("/fetch", rbac.RequirePermission("knowledge.read"), h.ListArticles)
		g.POST("/save", rbac.RequirePermission("knowledge.write"), h.Save)
		g.POST("/drop", rbac.RequirePermission("knowledge.write"), h.Drop)
		g.POST("/show", rbac.RequirePermission("knowledge.write"), h.ToggleShow)
	}
}

// RegisterUserRoutes registers user-facing routes under /me/knowledge
func (h *KnowledgeHandler) RegisterUserRoutes(me *gin.RouterGroup) {
	g := me.Group("/knowledge")
	{
		g.GET("/categories", h.UserListCategories)
		g.GET("/articles", h.UserListArticles)
		g.GET("/articles/:id", h.UserGetArticle)
	}
}

// ===== Admin =====

// ListCategories GET /admin/knowledge/getCategory
func (h *KnowledgeHandler) ListCategories(c *gin.Context) {
	items, err := h.knowledgeSvc.ListCategories(c.Request.Context())
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{"categories": items})
}

// ListArticles GET /admin/knowledge/fetch
func (h *KnowledgeHandler) ListArticles(c *gin.Context) {
	items, err := h.knowledgeSvc.ListArticles(c.Request.Context(), false)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{"items": items})
}

// Save POST /admin/knowledge/save
// Body: {type:"category", id, name, sort} for category
//
//	OR {category_id, id, title, body, show, sort} for article
func (h *KnowledgeHandler) Save(c *gin.Context) {
	var req model.SaveKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	if req.Type == "category" {
		cat, err := h.knowledgeSvc.SaveCategory(c.Request.Context(), req.ID, req.Name, req.Sort)
		if err != nil {
			server.Fail(c, 400, err.Error())
			return
		}
		server.OK(c, cat)
		return
	}
	// article
	a, err := h.knowledgeSvc.SaveArticle(c.Request.Context(), req.ID, req.CategoryID, req.Category, req.Title, req.Body, req.Show, req.Sort)
	if err != nil {
		server.Fail(c, 400, err.Error())
		return
	}
	server.OK(c, a)
}

// Drop POST /admin/knowledge/drop
func (h *KnowledgeHandler) Drop(c *gin.Context) {
	var req model.DropKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	if err := h.knowledgeSvc.DeleteCategoryOrArticle(c.Request.Context(), req.ID, req.Type); err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{"ok": true})
}

// ToggleShow POST /admin/knowledge/show
func (h *KnowledgeHandler) ToggleShow(c *gin.Context) {
	var req model.ShowKnowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	if err := h.knowledgeSvc.UpdateShow(c.Request.Context(), req.ID, req.Show); err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{"ok": true})
}

// ===== User =====

// UserListCategories GET /me/knowledge/categories
func (h *KnowledgeHandler) UserListCategories(c *gin.Context) {
	items, err := h.knowledgeSvc.ListCategories(c.Request.Context())
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{"categories": items})
}

// UserListArticles GET /me/knowledge/articles
func (h *KnowledgeHandler) UserListArticles(c *gin.Context) {
	items, err := h.knowledgeSvc.ListArticles(c.Request.Context(), true)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{"items": items})
}

// UserGetArticle GET /me/knowledge/articles/:id
func (h *KnowledgeHandler) UserGetArticle(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		server.ValidationError(c, "invalid id")
		return
	}
	// Use admin service but check show flag
	items, err := h.knowledgeSvc.ListArticles(c.Request.Context(), true)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	for _, a := range items {
		if a.ID == id {
			server.OK(c, a)
			return
		}
	}
	server.NotFound(c, "article not found")
}
