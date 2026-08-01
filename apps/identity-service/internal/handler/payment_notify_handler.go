package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
)

// PaymentNotifyHandler 处理第三方支付网关异步回调。
type PaymentNotifyHandler struct {
	paymentSvc *service.PaymentService
}

func NewPaymentNotifyHandler(paymentSvc *service.PaymentService) *PaymentNotifyHandler {
	return &PaymentNotifyHandler{paymentSvc: paymentSvc}
}

// Notify 易支付异步通知
// POST /api/v1/payment/notify/:method
func (h *PaymentNotifyHandler) Notify(c *gin.Context) {
	method := c.Param("method")
	if method != model.PaymentMethodWechat && method != model.PaymentMethodAlipay {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	params := map[string]string{}
	if err := c.Request.ParseForm(); err == nil {
		for k, vs := range c.Request.Form {
			if len(vs) > 0 {
				params[k] = vs[0]
			}
		}
	}
	if len(params) == 0 && c.Request.Body != nil {
		var m map[string]string
		if err := json.NewDecoder(c.Request.Body).Decode(&m); err == nil {
			for k, v := range m {
				params[k] = v
			}
		}
	}
	result, err := h.paymentSvc.HandleEpayNotify(c.Request.Context(), method, params)
	if err != nil {
		slog.Warn("epay notify rejected", "method", method, "error", err)
		c.String(http.StatusBadRequest, "fail")
		return
	}
	c.String(http.StatusOK, result)
}
