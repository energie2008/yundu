package lb

import "errors"

// 领域错误
var (
	// ErrPolicyNotFound 节点组未配置负载均衡策略（或策略不存在）
	ErrPolicyNotFound = errors.New("lb policy not found for group")
	// ErrNoCandidates 节点组下没有可用候选节点
	ErrNoCandidates = errors.New("no node candidates available for group")
	// ErrInvalidStrategy 不支持的负载均衡策略
	ErrInvalidStrategy = errors.New("invalid lb strategy")
	// ErrStickyMissingUserID sticky_user 策略缺少 user_id
	ErrStickyMissingUserID = errors.New("sticky_user strategy requires user_id")
	// ErrGeoMissingUserIP geo_affinity 策略缺少 user_ip
	ErrGeoMissingUserIP = errors.New("geo_affinity strategy requires user_ip")
)
