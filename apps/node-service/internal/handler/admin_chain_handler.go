package handler

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminChainHandler struct {
	chainService *service.ChainService
}

func NewAdminChainHandler(chainService *service.ChainService) *AdminChainHandler {
	return &AdminChainHandler{
		chainService: chainService,
	}
}

func (h *AdminChainHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	chains := admin.Group("/proxy-chains")
	{
		chains.POST("", rbac.RequirePermission("nodes.write"), h.CreateChain)
		chains.GET("", rbac.RequirePermission("nodes.read"), h.ListChains)
		chains.GET("/:id", rbac.RequirePermission("nodes.read"), h.GetChain)
		chains.POST("/:id/hops", rbac.RequirePermission("nodes.write"), h.AddHop)
		chains.DELETE("/:id/hops/:index", rbac.RequirePermission("nodes.write"), h.RemoveHop)
		chains.GET("/:id/hops", rbac.RequirePermission("nodes.read"), h.ListHops)
		chains.POST("/:id/bind", rbac.RequirePermission("nodes.write"), h.BindNode)
		chains.DELETE("/:id/bind/:node_id", rbac.RequirePermission("nodes.write"), h.UnbindNode)
	}
}

func (h *AdminChainHandler) CreateChain(c *gin.Context) {
	var req model.CreateChainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	chain, err := h.chainService.CreateChain(c.Request.Context(), &req)
	if err != nil {
		code, msg := service.MapChainErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.Created(c, model.NewProxyChainResponse(chain))
}

func (h *AdminChainHandler) ListChains(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := model.ChainStatus(c.Query("status"))

	chains, total, err := h.chainService.ListChains(c.Request.Context(), page, pageSize, status)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	items := make([]model.ProxyChainResponse, len(chains))
	for i, ch := range chains {
		items[i] = model.NewProxyChainResponse(ch)
	}

	server.OK(c, model.PaginationResponse{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    items,
	})
}

func (h *AdminChainHandler) GetChain(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid chain id")
		return
	}

	chain, err := h.chainService.GetChain(c.Request.Context(), id)
	if err != nil {
		code, msg := service.MapChainErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, model.NewProxyChainResponse(chain))
}

func (h *AdminChainHandler) AddHop(c *gin.Context) {
	idStr := c.Param("id")
	chainID, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid chain id")
		return
	}

	var req model.AddHopRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	hop, err := h.chainService.AddHop(c.Request.Context(), chainID, &req)
	if err != nil {
		code, msg := service.MapChainErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.Created(c, hop)
}

func (h *AdminChainHandler) RemoveHop(c *gin.Context) {
	idStr := c.Param("id")
	chainID, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid chain id")
		return
	}

	indexStr := c.Param("index")
	hopIndex, err := strconv.Atoi(indexStr)
	if err != nil {
		server.BadRequest(c, "invalid hop index")
		return
	}

	if err := h.chainService.RemoveHop(c.Request.Context(), chainID, hopIndex); err != nil {
		code, msg := service.MapChainErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.NoContent(c)
}

func (h *AdminChainHandler) ListHops(c *gin.Context) {
	idStr := c.Param("id")
	chainID, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid chain id")
		return
	}

	hops, err := h.chainService.ListHops(c.Request.Context(), chainID)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	server.OK(c, hops)
}

func (h *AdminChainHandler) BindNode(c *gin.Context) {
	idStr := c.Param("id")
	chainID, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid chain id")
		return
	}

	var req model.BindNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	if err := h.chainService.BindNode(c.Request.Context(), chainID, &req); err != nil {
		code, msg := service.MapChainErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{"status": "bound"})
}

func (h *AdminChainHandler) UnbindNode(c *gin.Context) {
	idStr := c.Param("id")
	chainID, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid chain id")
		return
	}

	nodeIDStr := c.Param("node_id")
	nodeID, err := uuid.Parse(nodeIDStr)
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}

	if err := h.chainService.UnbindNode(c.Request.Context(), chainID, nodeID); err != nil {
		code, msg := service.MapChainErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.NoContent(c)
}
