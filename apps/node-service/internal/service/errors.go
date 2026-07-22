package service

import "errors"

var (
	ErrServerAlreadyExists    = errors.New("server already exists")
	ErrServerNotFound         = errors.New("server not found")
	ErrCodeRequired           = errors.New("code is required")
	ErrRuntimeNotFound        = errors.New("runtime not found")
	ErrServerIDRequired       = errors.New("server_id is required")
	ErrRuntimeTypeRequired    = errors.New("runtime_type is required")
	ErrNodeAlreadyExists      = errors.New("node already exists")
	ErrNodeNotFound           = errors.New("node not found")
	ErrInvalidProtocolType    = errors.New("invalid protocol type")
	ErrConfigValidation       = errors.New("config validation failed")
	ErrChainAlreadyExists     = errors.New("proxy chain already exists")
	ErrChainNotFound          = errors.New("proxy chain not found")
	ErrHopNotFound            = errors.New("chain hop not found")
	ErrNodeAlreadyBound       = errors.New("node already bound to chain")
	ErrInvalidHopType         = errors.New("invalid hop type")
	ErrMaxHopsExceeded        = errors.New("max hops exceeded")
	ErrHealthStatusNotFound   = errors.New("health status not found")
	ErrVersionNotFound        = errors.New("config version not found")
	ErrBatchNotFound          = errors.New("deployment batch not found")
	ErrTargetNotFound         = errors.New("deployment target not found")
	ErrInvalidScopeType       = errors.New("invalid scope type")
	ErrDeploymentRunning      = errors.New("deployment already running")
	ErrInvalidContent         = errors.New("invalid content")
	ErrNodeGroupAlreadyExists = errors.New("node group already exists")
	ErrNodeGroupNotFound      = errors.New("node group not found")
	ErrNodeGroupInUse         = errors.New("node group is in use")
	// ErrPreflightValidation 发布前四级校验失败（L1 Schema / L2 语义 / L3 能力矩阵 / L3.5 TLSMaterial）
	ErrPreflightValidation = errors.New("preflight validation failed")
	// ErrPayloadNotFound P3-1: 加密 Payload Manifest 不存在
	ErrPayloadNotFound = errors.New("payload manifest not found")
)
