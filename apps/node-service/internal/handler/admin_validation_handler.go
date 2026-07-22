package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/curve25519"
	"gopkg.in/yaml.v3"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/airport-panel/subscription/nodespec"
	"github.com/airport-panel/subscription/validator"
)

// AdminValidationHandler 节点校验 handler
// 注入 DualKernelValidator 后，校验链路为：
//  1. spec.Validate() — 基础字段校验（快速失败）
//  2. DualKernelValidator.ValidateBoth — Enhancement 专项 → 双核渲染 → dry-run → 语义等价性
type AdminValidationHandler struct {
	validator *validator.DualKernelValidator
}

type NodeValidationRequest struct {
	RawYAML string             `json:"raw_yaml,omitempty"`
	Spec    *nodespec.NodeSpec `json:"spec,omitempty"`
}

// NodeValidationResponse 校验响应
// 保留旧字段（status/xray_valid/singbox_valid 等）向后兼容，
// 新增 errors 数组（含 level/kernel/field）和渲染后的双核配置供前端调试。
type NodeValidationResponse struct {
	Status        string             `json:"status"` // "valid" | "invalid"
	XrayValid     bool               `json:"xray_valid"`
	SingBoxValid  bool               `json:"singbox_valid"`
	Error         string             `json:"error,omitempty"`
	XrayError     string             `json:"xray_error,omitempty"`
	SingBoxError  string             `json:"singbox_error,omitempty"`
	Errors        []validator.ValidationError `json:"errors,omitempty"`
	XrayConfig    map[string]any     `json:"xray_config,omitempty"`
	SingBoxConfig map[string]any     `json:"singbox_config,omitempty"`
}

// NewAdminValidationHandler 构造校验 handler
// validator 为 nil 时退化为仅做 spec.Validate() + 基础渲染（不推荐，仅用于兜底）
func NewAdminValidationHandler(v *validator.DualKernelValidator) *AdminValidationHandler {
	return &AdminValidationHandler{validator: v}
}

func (h *AdminValidationHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	nodes := admin.Group("/nodes")
	{
		nodes.POST("/validate", rbac.RequirePermission("nodes.write"), h.ValidateNode)
		nodes.POST("/reality-keypair", rbac.RequirePermission("nodes.write"), h.GenerateRealityKeypair)
	}
}

func (h *AdminValidationHandler) ValidateNode(c *gin.Context) {
	var req NodeValidationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, "invalid request body: "+err.Error())
		return
	}

	var spec *nodespec.NodeSpec

	if req.RawYAML != "" {
		spec = &nodespec.NodeSpec{}
		if err := yaml.Unmarshal([]byte(req.RawYAML), spec); err != nil {
			server.BadRequest(c, "failed to parse YAML: "+err.Error())
			return
		}
	} else if req.Spec != nil {
		spec = req.Spec
	} else {
		server.BadRequest(c, "either raw_yaml or spec is required")
		return
	}

	// 第1步：基础字段校验（快速失败，不走双核渲染）
	if err := spec.Validate(); err != nil {
		resp := NodeValidationResponse{
			Status:    "invalid",
			XrayValid: false,
			SingBoxValid: false,
			Error:     err.Error(),
			Errors: []validator.ValidationError{{
				Level:   validator.LevelError,
				Message: err.Error(),
			}},
		}
		server.OK(c, resp)
		return
	}

	// 第2步：DualKernelValidator 四步校验
	// Enhancement 专项 → 双核渲染 → 真实 dry-run（如有二进制）→ 语义等价性
	if h.validator == nil {
		// 兜底：无 validator 时只返回基础校验通过
		server.OK(c, NodeValidationResponse{
			Status:        "valid",
			XrayValid:     true,
			SingBoxValid:  true,
		})
		return
	}

	result := h.validator.ValidateBoth(c.Request.Context(), spec)

	// 将 ValidationResult 转换为响应
	resp := NodeValidationResponse{
		Status:        "valid",
		XrayValid:     true,
		SingBoxValid:  true,
		Errors:        result.Errors,
		XrayConfig:    result.XrayConfig,
		SingBoxConfig: result.SingBoxConfig,
	}

	// 按 kernel 分拣 error/warning，填充兼容字段
	for _, e := range result.Errors {
		if e.Level == validator.LevelError {
			resp.Status = "invalid"
			if e.Kernel == "xray" {
				resp.XrayValid = false
				if resp.XrayError == "" {
					resp.XrayError = e.Message
				} else {
					resp.XrayError += "; " + e.Message
				}
			} else if e.Kernel == "sing_box" {
				resp.SingBoxValid = false
				if resp.SingBoxError == "" {
					resp.SingBoxError = e.Message
				} else {
					resp.SingBoxError += "; " + e.Message
				}
			} else {
				// 通用 error（非特定内核）
				if resp.Error == "" {
					resp.Error = e.Message
				} else {
					resp.Error += "; " + e.Message
				}
			}
		}
	}

	// 如果只有 warning 没有 error，状态仍为 valid

	server.OK(c, resp)
}

// RealityKeypairResponse REALITY 密钥对生成响应
type RealityKeypairResponse struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
	ShortID    string `json:"short_id"`
}

// GenerateRealityKeypair 生成 REALITY x25519 密钥对 + short_id
// 用途：面板创建/编辑 REALITY 节点时一键生成密钥，无需 SSH 到 VPS 执行 xray x25519
// 算法：crypto/rand 生成 32 字节私钥 → curve25519.X25519 派生公钥 → base64url 编码
func (h *AdminValidationHandler) GenerateRealityKeypair(c *gin.Context) {
	// 1. 生成 32 字节随机私钥
	privKey := make([]byte, 32)
	if _, err := rand.Read(privKey); err != nil {
		server.InternalError(c, "生成私钥失败: "+err.Error())
		return
	}
	// curve25519 私钥 clamping（RFC 7748）
	privKey[0] &= 248
	privKey[31] &= 127
	privKey[31] |= 64

	// 2. 派生公钥
	pubKey, err := curve25519.X25519(privKey, curve25519.Basepoint)
	if err != nil {
		server.InternalError(c, "派生公钥失败: "+err.Error())
		return
	}

	// 3. 生成 4 字节 short_id（hex 编码 = 8 字符）
	shortIDBytes := make([]byte, 4)
	if _, err := rand.Read(shortIDBytes); err != nil {
		server.InternalError(c, "生成 short_id 失败: "+err.Error())
		return
	}

	resp := RealityKeypairResponse{
		PrivateKey: base64.RawURLEncoding.EncodeToString(privKey),
		PublicKey:  base64.RawURLEncoding.EncodeToString(pubKey),
		ShortID:    hex.EncodeToString(shortIDBytes),
	}
	server.OK(c, resp)
}
