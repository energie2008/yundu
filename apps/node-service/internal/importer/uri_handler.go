package importer

import (
	"github.com/airport-panel/config/server"
	"github.com/gin-gonic/gin"
)

func (h *AdminImportHandler) PreviewImportURI(c *gin.Context) {
	var req ImportURIPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	resp, err := h.svc.ImportURIs(c.Request.Context(), &req)
	if err != nil {
		code, msg := MapImporterErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, resp)
}

func (h *AdminImportHandler) ConfirmImportURI(c *gin.Context) {
	var req ImportURIConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	resp, err := h.svc.ConfirmImportURIs(c.Request.Context(), &req)
	if err != nil {
		code, msg := MapImporterErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, resp)
}
