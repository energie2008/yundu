package kernelrender

import "strings"

// StreamingPreset 描述单个流媒体平台的解锁规则集。
//
// 字段语义对齐 sing-box / xray 路由规则：
//   - Domains:  精确域名后缀，对应 sing-box domain_suffix / xray domain:
//   - Keywords: 域名子串，    对应 sing-box domain_keyword / xray keyword:
//   - Geosite:  geosite 分类，对应 sing-box geosite     / xray geosite:（需 geosite 数据库）
//
// 该结构仅承载“匹配哪些域名”，不绑定出站 tag。
// 实际出站 tag（outboundTag / outbound）由调用方根据节点路由策略填充，
// 这样同一份预置可被多套路由复用。
type StreamingPreset struct {
	Name     string   // 平台展示名（如 "Netflix"）
	Domains  []string // 域名后缀列表（domain_suffix）
	Keywords []string // 域名关键字列表（domain_keyword）
	Geosite  []string // geosite 分类列表（可选，需 geosite 数据库支持）
}

// StreamingPresets 是流媒体解锁预置规则集的全局注册表。
//
// key 为平台的小写短标识（如 "netflix"），value 为该平台的规则定义。
// 新增平台只需在此追加一项，GetStreamingPresets / GetStreamingPresetByName
// 会自动覆盖，无需改动调用方。
var StreamingPresets = map[string]StreamingPreset{
	"netflix": {
		Name:     "Netflix",
		Domains:  []string{"netflix.com", "nflxvideo.net", "nflximg.net", "nflxext.com", "netflix.net", "netflixso.net"},
		Keywords: []string{"netflix", "nflx"},
		Geosite:  []string{"netflix"},
	},
	"disney": {
		Name:     "Disney+",
		Domains:  []string{"disneyplus.com", "disney-plus.net", "dssott.com", "bamgrid.com", "cdn.registerdisney.go.com"},
		Keywords: []string{"disney"},
		Geosite:  []string{"disney"},
	},
	"youtube": {
		Name:     "YouTube Premium",
		Domains:  []string{"youtube.com", "googlevideo.com", "youtubei.googleapis.com", "ytimg.com", "youtu.be", "m.youtube.com"},
		Keywords: []string{"youtube"},
		Geosite:  []string{"youtube"},
	},
	"openai": {
		Name:     "ChatGPT/OpenAI",
		Domains:  []string{"openai.com", "chatgpt.com", "oaistatic.com", "oaiusercontent.com", "cdn.oaistatic.com"},
		Keywords: []string{"openai", "chatgpt"},
		Geosite:  []string{"openai"},
	},
	"tiktok": {
		Name:     "TikTok",
		Domains:  []string{"tiktok.com", "tiktokcdn.com", "tiktokv.com", "musical.ly", "tiktokcdn-us.com", "byteoversea.com", "ibyteimg.com"},
		Keywords: []string{"tiktok"},
		Geosite:  []string{"tiktok"},
	},
	"hbo": {
		Name:     "HBO Max",
		Domains:  []string{"hbomax.com", "max.com", "hbonow.com", "hbogo.com", "hbo.com", "hbomaxcdn.com", "players.hbomaxcdn.com"},
		Keywords: []string{"hbo"},
		Geosite:  []string{"hbo"},
	},
	"spotify": {
		Name:     "Spotify",
		Domains:  []string{"spotify.com", "scdn.co", "spotifycdn.com", "apresolve.spotify.com", "spclient.wg.spotify.com"},
		Keywords: []string{"spotify"},
		Geosite:  []string{"spotify"},
	},
	"primevideo": {
		Name:     "Prime Video",
		Domains:  []string{"primevideo.com", "amazonvideo.com", "aiv-cdn.net", "atv-ps.amazon.com", "fls-na.amazon.com"},
		Keywords: []string{"primevideo", "amazonvideo"},
		Geosite:  []string{"amazon"},
	},
	"bilibili": {
		// Bilibili 港澳台：匹配 bilibili 全量域名，调用方需配合 geoip cn 排除大陆，
		// 仅将港澳台访问的请求路由到解锁节点。
		Name:     "Bilibili 港澳台",
		Domains:  []string{"bilibili.com", "bilibili.hk", "bilibili.tv", "api.bilibili.com", "passport.bilibili.com", "app.bilibili.com"},
		Keywords: []string{"bilibili"},
		Geosite:  []string{"bilibili"},
	},
}

// ToXrayRules 生成 Xray 路由规则（type:field + domain[]）。
//
// Xray 的 domain 字段是带前缀的字符串数组：
//   - "domain:example.com"  域名后缀匹配
//   - "keyword:foo"         子串匹配
//   - "geosite:netflix"     geosite 分类匹配
//
// outboundTag 留空，由调用方填充实际出站 tag。
func (p StreamingPreset) ToXrayRules() []map[string]interface{} {
	domains := make([]string, 0, len(p.Domains)+len(p.Geosite)+len(p.Keywords))
	for _, d := range p.Domains {
		domains = append(domains, "domain:"+d)
	}
	for _, g := range p.Geosite {
		domains = append(domains, "geosite:"+g)
	}
	for _, k := range p.Keywords {
		domains = append(domains, "keyword:"+k)
	}
	return []map[string]interface{}{
		{
			"type":        "field",
			"domain":      domains,
			"outboundTag": "", // 由调用方填充实际出站 tag
		},
	}
}

// ToSingBoxRules 生成 sing-box 路由规则（domain_suffix / domain_keyword / geosite）。
//
// sing-box 使用独立字段承载不同匹配类型，比 xray 的前缀字符串更结构化。
// outbound 留空，由调用方填充实际出站 tag。
func (p StreamingPreset) ToSingBoxRules() []map[string]interface{} {
	rule := map[string]interface{}{}
	if len(p.Domains) > 0 {
		rule["domain_suffix"] = p.Domains
	}
	if len(p.Keywords) > 0 {
		rule["domain_keyword"] = p.Keywords
	}
	if len(p.Geosite) > 0 {
		rule["geosite"] = p.Geosite
	}
	return []map[string]interface{}{rule}
}

// ToRules 将预置规则转换为完整的路由规则结构（同时包含 xray 与 sing-box 两种格式）。
//
// 返回的 map 结构：
//
//	{
//	  "name":     "Netflix",
//	  "domains":  []string{...},   // 原始域名后缀
//	  "keywords": []string{...},   // 原始关键字
//	  "geosite":  []string{...},   // 原始 geosite 分类
//	  "xray":     []map[string]interface{}{ {type:field, domain:[...], outboundTag:""} },
//	  "sing_box": []map[string]interface{}{ {domain_suffix:[...], domain_keyword:[...], geosite:[...]} },
//	}
func (p StreamingPreset) ToRules() map[string]interface{} {
	return map[string]interface{}{
		"name":     p.Name,
		"domains":  p.Domains,
		"keywords": p.Keywords,
		"geosite":  p.Geosite,
		"xray":     p.ToXrayRules(),
		"sing_box": p.ToSingBoxRules(),
	}
}

// GetStreamingPresets 返回所有流媒体平台预置规则集。
//
// 返回类型 map[string]map[string]interface{}：
//   - 外层 key   为平台短标识（如 "netflix"）
//   - 内层 value 为该平台的路由规则（见 StreamingPreset.ToRules）
//
// 每次调用都重新构建 map，调用方可安全修改返回值而不影响全局 StreamingPresets。
func GetStreamingPresets() map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{}, len(StreamingPresets))
	for key, preset := range StreamingPresets {
		result[key] = preset.ToRules()
	}
	return result
}

// GetStreamingPresetByName 按名称获取单个流媒体预置规则集。
//
// 查找顺序：
//  1. 按短标识精确匹配（如 "netflix"）
//  2. 按展示名不区分大小写匹配（如 "Netflix"、"DISNEY+"）
//
// 找到时返回 (规则, true)；否则返回 (nil, false)。
func GetStreamingPresetByName(name string) (map[string]interface{}, bool) {
	// 1. 按短标识精确匹配
	if preset, ok := StreamingPresets[name]; ok {
		return preset.ToRules(), true
	}
	// 2. 按展示名不区分大小写匹配
	lower := strings.ToLower(name)
	for _, preset := range StreamingPresets {
		if strings.ToLower(preset.Name) == lower {
			return preset.ToRules(), true
		}
	}
	return nil, false
}
