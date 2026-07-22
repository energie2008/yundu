// Package hotdiff 实现配置变更的四级分类器，用于 node-agent 端决定
// 对 xray/sing-box 配置变更采取何种热重载策略。
//
// 分类优先级（从高到低）：
//   RESTART_REQUIRED  > HOT_TLS_RELOAD > HOT_ROUTING_ONLY > HOT_USER_ONLY
package hotdiff

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// DiffLevel 表示配置变更的分类级别。
type DiffLevel string

const (
	// DiffHotUserOnly 仅用户变更 → AlterInbound 热重载（不重启）
	DiffHotUserOnly DiffLevel = "HOT_USER_ONLY"
	// DiffHotRoutingOnly 仅路由规则变更 → ReloadRouting 热重载（不重启）
	DiffHotRoutingOnly DiffLevel = "HOT_ROUTING_ONLY"
	// DiffHotTLSReload TLS/证书变更 → ReloadTLS（不重启，SIGUSR1）
	DiffHotTLSReload DiffLevel = "HOT_TLS_RELOAD"
	// DiffRestartNeeded 协议/端口/传输层变更 → 30s 防抖重启
	DiffRestartNeeded DiffLevel = "RESTART_REQUIRED"
)

// DiffDetail 包含变更分类结果与摘要信息，用于日志记录。
type DiffDetail struct {
	Level         DiffLevel
	ChangedFields []string // 变更字段列表，如 ["inbounds[tag1].settings.clients", "routing.rules"]
	Summary       string   // 人类可读摘要
	// UserChanges P1-5: 结构化用户变更明细，仅在 Level == HOT_USER_ONLY 时填充。
	// 调用方（applyConfig）据此调用 executor.AlterInbound 进行真增量热重载。
	UserChanges []UserChange
}

// UserOp 表示用户变更操作类型。
type UserOp string

const (
	UserOpAdded    UserOp = "added"
	UserOpRemoved  UserOp = "removed"
	UserOpModified UserOp = "modified"
)

// UserChange 描述单个用户的变更明细（P1-5）。
type UserChange struct {
	InboundTag string                 // 所属 inbound 的 tag
	Email      string                 // 用户 email（xray client 的唯一键）
	Op         UserOp                 // 变更操作
	Account    map[string]interface{} // 新的 client 对象（added/modified 时填充，removed 时为 nil）
}

// changeCollector 按级别收集变更字段。
type changeCollector struct {
	restart    []string
	tls        []string
	routing    []string
	user       []string
	userChange []UserChange // P1-5: 结构化用户变更
}

// ComputeHotDiff 比较新旧两份 xray/sing-box 配置（JSON map），返回变更分类结果。
// oldCfg 为 nil 表示首次部署；两者均为空则视为无变更。
func ComputeHotDiff(oldCfg, newCfg map[string]interface{}) DiffDetail {
	if isEmptyConfig(oldCfg) && isEmptyConfig(newCfg) {
		return DiffDetail{}
	}
	// 首次部署 → restart
	if isEmptyConfig(oldCfg) {
		return DiffDetail{
			Level:         DiffRestartNeeded,
			ChangedFields: []string{"initial deployment"},
			Summary:       "initial config deployment, restart required",
		}
	}
	// 配置被清空 → restart
	if isEmptyConfig(newCfg) {
		return DiffDetail{
			Level:         DiffRestartNeeded,
			ChangedFields: []string{"config removed"},
			Summary:       "config removed, restart required",
		}
	}

	var c changeCollector
	compareInbounds(asSlice(oldCfg["inbounds"]), asSlice(newCfg["inbounds"]), &c)
	compareRouting(asMap(oldCfg["routing"]), asMap(newCfg["routing"]), &c)
	compareOutbounds(asSlice(oldCfg["outbounds"]), asSlice(newCfg["outbounds"]), &c)
	compareOtherTopLevel(oldCfg, newCfg, &c)

	return buildResult(&c)
}

// --- inbounds 比较 ---

func compareInbounds(oldInbounds, newInbounds []interface{}, c *changeCollector) {
	oldMap := indexInboundsByTag(oldInbounds)
	newMap := indexInboundsByTag(newInbounds)

	// 节点新增或删除 → restart
	for tag := range oldMap {
		if _, ok := newMap[tag]; !ok {
			c.restart = append(c.restart, fmt.Sprintf("inbounds[%s] removed", tag))
		}
	}
	for tag := range newMap {
		if _, ok := oldMap[tag]; !ok {
			c.restart = append(c.restart, fmt.Sprintf("inbounds[%s] added", tag))
		}
	}

	// tag 匹配的节点逐字段比较
	for tag, oldInb := range oldMap {
		if newInb, ok := newMap[tag]; ok {
			compareInbound(tag, oldInb, newInb, c)
		}
	}
}

func compareInbound(tag string, oldInb, newInb map[string]interface{}, c *changeCollector) {
	prefix := fmt.Sprintf("inbounds[%s]", tag)

	// port 变更 → restart
	if !reflect.DeepEqual(oldInb["port"], newInb["port"]) {
		c.restart = append(c.restart, prefix+".port")
	}
	// protocol 变更 → restart
	if !reflect.DeepEqual(oldInb["protocol"], newInb["protocol"]) {
		c.restart = append(c.restart, prefix+".protocol")
	}

	compareStreamSettings(prefix, asMap(oldInb["streamSettings"]), asMap(newInb["streamSettings"]), c)
	compareClients(tag, prefix, asMap(oldInb["settings"]), asMap(newInb["settings"]), c)
	compareInboundOtherFields(prefix, oldInb, newInb, c)
}

// compareInboundOtherFields 处理 inbound 中未明确归类的字段变更，保守视为 restart。
func compareInboundOtherFields(prefix string, oldInb, newInb map[string]interface{}, c *changeCollector) {
	known := map[string]bool{"tag": true, "port": true, "protocol": true, "streamSettings": true, "settings": true}
	for k := range unionKeys(oldInb, newInb) {
		if known[k] {
			continue
		}
		if !reflect.DeepEqual(oldInb[k], newInb[k]) {
			c.restart = append(c.restart, prefix+"."+k)
		}
	}
}

// --- streamSettings 比较 ---

func compareStreamSettings(prefix string, oldSS, newSS map[string]interface{}, c *changeCollector) {
	ssPrefix := prefix + ".streamSettings"

	// network（传输协议）变更 → restart
	if !reflect.DeepEqual(oldSS["network"], newSS["network"]) {
		c.restart = append(c.restart, ssPrefix+".network")
	}

	// security 从无到有或从有到无（none↔tls/reality）→ restart
	oldSec := toString(oldSS["security"])
	newSec := toString(newSS["security"])
	oldHasSec := oldSec != "" && oldSec != "none"
	newHasSec := newSec != "" && newSec != "none"
	if oldHasSec != newHasSec {
		c.restart = append(c.restart, ssPrefix+".security")
	}

	// reality 变更（serverName/privateKey/publicKey 等）→ restart
	oldReality := asMap(oldSS["reality"])
	newReality := asMap(newSS["reality"])
	if !reflect.DeepEqual(oldReality, newReality) {
		c.restart = append(c.restart, ssPrefix+".reality")
	}

	// TLS certificates 变更 → TLS reload（reality 不属于此类）
	oldTLS := asMap(oldSS["tls"])
	newTLS := asMap(newSS["tls"])
	if !reflect.DeepEqual(oldTLS["certificates"], newTLS["certificates"]) {
		c.tls = append(c.tls, ssPrefix+".tls.certificates")
	}

	// 其他 streamSettings 字段（如 wsSettings/tcpSettings 路径变更）→ restart
	knownSS := map[string]bool{"network": true, "security": true, "reality": true, "tls": true}
	for k := range unionKeys(oldSS, newSS) {
		if knownSS[k] {
			continue
		}
		if !reflect.DeepEqual(oldSS[k], newSS[k]) {
			c.restart = append(c.restart, ssPrefix+"."+k)
		}
	}
}

// --- clients/users 比较 ---

func compareClients(tag, prefix string, oldSettings, newSettings map[string]interface{}, c *changeCollector) {
	oldByEmail := indexClientsByEmail(getClientArray(oldSettings))
	newByEmail := indexClientsByEmail(getClientArray(newSettings))

	// 删除的用户
	for email := range oldByEmail {
		if _, ok := newByEmail[email]; !ok {
			c.user = append(c.user, fmt.Sprintf("%s.settings.clients[%s] removed", prefix, email))
			c.userChange = append(c.userChange, UserChange{
				InboundTag: tag,
				Email:      email,
				Op:         UserOpRemoved,
				Account:    nil,
			})
		}
	}
	// 新增的用户
	for email, newClient := range newByEmail {
		if _, ok := oldByEmail[email]; !ok {
			c.user = append(c.user, fmt.Sprintf("%s.settings.clients[%s] added", prefix, email))
			c.userChange = append(c.userChange, UserChange{
				InboundTag: tag,
				Email:      email,
				Op:         UserOpAdded,
				Account:    newClient,
			})
		}
	}
	// 修改的用户（email 匹配但内容不同）
	for email, oldClient := range oldByEmail {
		if newClient, ok := newByEmail[email]; ok {
			if !reflect.DeepEqual(oldClient, newClient) {
				c.user = append(c.user, fmt.Sprintf("%s.settings.clients[%s] modified", prefix, email))
				c.userChange = append(c.userChange, UserChange{
					InboundTag: tag,
					Email:      email,
					Op:         UserOpModified,
					Account:    newClient,
				})
			}
		}
	}

	// settings 中其他字段（如 decryption/fallbacks）变更 → restart
	knownSettings := map[string]bool{"clients": true, "users": true}
	for k := range unionKeys(oldSettings, newSettings) {
		if knownSettings[k] {
			continue
		}
		if !reflect.DeepEqual(oldSettings[k], newSettings[k]) {
			c.restart = append(c.restart, prefix+".settings."+k)
		}
	}
}

// getClientArray 兼容 xray 的 clients 和 sing-box 的 users 字段。
func getClientArray(settings map[string]interface{}) []interface{} {
	if clients := asSlice(settings["clients"]); clients != nil {
		return clients
	}
	return asSlice(settings["users"])
}

// --- routing 比较 ---

func compareRouting(oldRouting, newRouting map[string]interface{}, c *changeCollector) {
	if oldRouting == nil && newRouting == nil {
		return
	}

	// rules 变更 → routing
	if !reflect.DeepEqual(oldRouting["rules"], newRouting["rules"]) {
		c.routing = append(c.routing, "routing.rules")
	}
	// domainStrategy 变更 → routing
	if !reflect.DeepEqual(oldRouting["domainStrategy"], newRouting["domainStrategy"]) {
		c.routing = append(c.routing, "routing.domainStrategy")
	}

	// 其他 routing 字段（如 balancers）变更 → restart
	known := map[string]bool{"rules": true, "domainStrategy": true}
	for k := range unionKeys(oldRouting, newRouting) {
		if known[k] {
			continue
		}
		if !reflect.DeepEqual(oldRouting[k], newRouting[k]) {
			c.restart = append(c.restart, "routing."+k)
		}
	}
}

// --- outbounds 比较 ---

func compareOutbounds(oldOutbounds, newOutbounds []interface{}, c *changeCollector) {
	if !reflect.DeepEqual(oldOutbounds, newOutbounds) {
		c.routing = append(c.routing, "outbounds")
	}
}

// --- 顶层其他字段 ---

// compareOtherTopLevel 处理 inbounds/routing/outbounds 之外的顶层字段变更，保守视为 restart。
func compareOtherTopLevel(oldCfg, newCfg map[string]interface{}, c *changeCollector) {
	known := map[string]bool{"inbounds": true, "routing": true, "outbounds": true}
	for k := range unionKeys(oldCfg, newCfg) {
		if known[k] {
			continue
		}
		if !reflect.DeepEqual(oldCfg[k], newCfg[k]) {
			c.restart = append(c.restart, k)
		}
	}
}

// --- 结果汇总 ---

func buildResult(c *changeCollector) DiffDetail {
	all := make([]string, 0, len(c.restart)+len(c.tls)+len(c.routing)+len(c.user))
	all = append(all, c.restart...)
	all = append(all, c.tls...)
	all = append(all, c.routing...)
	all = append(all, c.user...)
	sort.Strings(all)

	switch {
	case len(c.restart) > 0:
		return DiffDetail{
			Level:         DiffRestartNeeded,
			ChangedFields: all,
			Summary:       fmt.Sprintf("restart required: %s", strings.Join(c.restart, ", ")),
		}
	case len(c.tls) > 0:
		return DiffDetail{
			Level:         DiffHotTLSReload,
			ChangedFields: all,
			Summary:       fmt.Sprintf("tls reload: %s", strings.Join(c.tls, ", ")),
		}
	case len(c.routing) > 0:
		return DiffDetail{
			Level:         DiffHotRoutingOnly,
			ChangedFields: all,
			Summary:       fmt.Sprintf("routing reload: %s", strings.Join(c.routing, ", ")),
		}
	case len(c.user) > 0:
		return DiffDetail{
			Level:         DiffHotUserOnly,
			ChangedFields: all,
			Summary:       fmt.Sprintf("user reload: %s", strings.Join(c.user, ", ")),
			UserChanges:   c.userChange,
		}
	default:
		return DiffDetail{}
	}
}

// --- 辅助函数 ---

func isEmptyConfig(cfg map[string]interface{}) bool {
	return cfg == nil || len(cfg) == 0
}

func asMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}

func asSlice(v interface{}) []interface{} {
	if s, ok := v.([]interface{}); ok {
		return s
	}
	return nil
}

func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// unionKeys 返回两个 map 的键并集。
func unionKeys(a, b map[string]interface{}) map[string]bool {
	keys := make(map[string]bool)
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	return keys
}

// indexInboundsByTag 按 tag 字段索引 inbound，tag 缺失时回退到 untagged-{index}。
func indexInboundsByTag(inbounds []interface{}) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{})
	for i, inb := range inbounds {
		m := asMap(inb)
		if m == nil {
			continue
		}
		tag := toString(m["tag"])
		if tag == "" {
			tag = fmt.Sprintf("untagged-%d", i)
		}
		result[tag] = m
	}
	return result
}

// indexClientsByEmail 按 email 字段索引用户，email 缺失时回退到 no-email-{index}。
func indexClientsByEmail(clients []interface{}) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{})
	for i, cl := range clients {
		m := asMap(cl)
		if m == nil {
			continue
		}
		email := toString(m["email"])
		if email == "" {
			email = fmt.Sprintf("no-email-%d", i)
		}
		result[email] = m
	}
	return result
}
