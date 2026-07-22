package handler

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/middleware"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminCommissionHandler 返利/提现管理（管理端）
type AdminCommissionHandler struct {
	commissionSvc *service.CommissionService
}

func NewAdminCommissionHandler(commissionSvc *service.CommissionService) *AdminCommissionHandler {
	return &AdminCommissionHandler{commissionSvc: commissionSvc}
}

func (h *AdminCommissionHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	g := admin.Group("/commissions")
	{
		g.GET("/withdrawals", rbac.RequirePermission("finance.read"), h.ListWithdrawals)
		g.POST("/withdrawals/:id/approve", rbac.RequirePermission("finance.write"), h.ApproveWithdrawal)
		g.POST("/withdrawals/:id/reject", rbac.RequirePermission("finance.write"), h.RejectWithdrawal)
	}
}

// ListWithdrawals GET /admin/commissions/withdrawals?status=0|1|2
func (h *AdminCommissionHandler) ListWithdrawals(c *gin.Context) {
	var statusFilter *int
	if s := c.Query("status"); s != "" {
		v, err := strconv.Atoi(s)
		if err == nil {
			statusFilter = &v
		}
	}
	items, total, err := h.commissionSvc.ListAllWithdrawals(c.Request.Context(), statusFilter)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	resp := make([]model.WithdrawResponse, len(items))
	for i, w := range items {
		resp[i] = model.NewWithdrawResponse(w)
	}
	server.OK(c, gin.H{"items": resp, "total": total})
}

// ApproveWithdrawal POST /admin/commissions/withdrawals/:id/approve
func (h *AdminCommissionHandler) ApproveWithdrawal(c *gin.Context) {
	idStr := c.Param("id")
	wid, err := uuid.Parse(idStr)
	if err != nil {
		server.ValidationError(c, "invalid withdrawal id")
		return
	}
	adminID := middleware.GetAdminID(c)
	if adminID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	var body struct {
		Remark string `json:"remark"`
	}
	_ = c.ShouldBindJSON(&body)
	w, err := h.commissionSvc.ProcessWithdrawal(c.Request.Context(), wid, adminID, true, body.Remark)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, model.NewWithdrawResponse(w))
}

// RejectWithdrawal POST /admin/commissions/withdrawals/:id/reject
func (h *AdminCommissionHandler) RejectWithdrawal(c *gin.Context) {
	idStr := c.Param("id")
	wid, err := uuid.Parse(idStr)
	if err != nil {
		server.ValidationError(c, "invalid withdrawal id")
		return
	}
	adminID := middleware.GetAdminID(c)
	if adminID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	var body struct {
		Remark string `json:"remark"`
	}
	_ = c.ShouldBindJSON(&body)
	w, err := h.commissionSvc.ProcessWithdrawal(c.Request.Context(), wid, adminID, false, body.Remark)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, model.NewWithdrawResponse(w))
}
