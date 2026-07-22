package routing

import (
	"errors"

	"github.com/airport-panel/config"
)

// 领域错误
var (
	// RouteRuleSet 错误
	ErrRuleSetNotFound      = errors.New("route rule set not found")
	ErrRuleSetDuplicateCode = errors.New("route rule set code already exists")
	ErrBuiltinCannotDelete  = errors.New("builtin rule set cannot be deleted")
	ErrRuleSetNoSourceURL    = errors.New("rule set has no source_url to sync")
	ErrRuleSetSyncFailed     = errors.New("failed to sync rule set from remote url")

	// RoutePolicy 错误
	ErrPolicyNotFound      = errors.New("route policy not found")
	ErrPolicyDuplicateCode = errors.New("route policy code already exists")
	ErrTemplateCannotDelete  = errors.New("builtin template policy cannot be deleted")
	ErrTemplateNotFound      = errors.New("route policy template not found")

	// RoutePolicyRule 错误
	ErrPolicyRuleNotFound    = errors.New("route policy rule not found")
	ErrInvalidOutboundAction = errors.New("invalid outbound_action: tag/balancer requires outbound_tag")
	ErrInvalidRuleSource     = errors.New("invalid rule_source: rule_set requires rule_set_id")

	// NodeRouteBinding 错误
	ErrBindingNotFound   = errors.New("node route binding not found")
	ErrBindingDuplicate  = errors.New("node route binding already exists")

	// NodeGroupLBPolicy 错误
	ErrLBPolicyNotFound = errors.New("node group lb policy not found")

	// OutboundGroup 错误
	ErrOutboundGroupNotFound   = errors.New("outbound group not found")
	ErrOutboundGroupDuplicate  = errors.New("outbound group tag already exists for this node")

	// 渲染错误
	ErrRenderFailed = errors.New("failed to render routing config")
)

// MapRoutingErrorToCode 将 routing 包的错误映射到 ErrorCode
func MapRoutingErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrRuleSetNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrRuleSetDuplicateCode):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrBuiltinCannotDelete):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrRuleSetNoSourceURL):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrRuleSetSyncFailed):
		return config.CodeInternalError, err.Error()
	case errors.Is(err, ErrPolicyNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrPolicyDuplicateCode):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrTemplateCannotDelete):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrTemplateNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrPolicyRuleNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrInvalidOutboundAction):
		return config.CodeValidationFailed, err.Error()
	case errors.Is(err, ErrInvalidRuleSource):
		return config.CodeValidationFailed, err.Error()
	case errors.Is(err, ErrBindingNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrBindingDuplicate):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrLBPolicyNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrOutboundGroupNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrOutboundGroupDuplicate):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrRenderFailed):
		return config.CodeInternalError, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}
