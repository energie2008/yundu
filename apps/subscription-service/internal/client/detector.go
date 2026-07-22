package client

import (
	"strings"

	"github.com/airport-panel/subscription-service/internal/model"
)

type ClientInfo struct {
	Type    model.ClientType
	OS      string
	IsMeta  bool
	Version string
}

func DetectClient(userAgent string) model.ClientType {
	return DetectClientEx(userAgent).Type
}

func DetectClientEx(userAgent string) ClientInfo {
	ua := strings.ToLower(userAgent)
	info := ClientInfo{Type: model.ClientTypeURI}

	if ua == "" {
		return info
	}

	switch {
	case containsAny(ua, "mihomo-party"):
		info.Type = model.ClientTypeMihomoParty
		info.IsMeta = true
	case containsAny(ua, "mihomo", "clash-meta", "clashmeta", "clash.meta", "meta"):
		info.Type = model.ClientTypeMihomo
		info.IsMeta = true
	case containsAny(ua, "verge-rev", "verge rev"):
		info.Type = model.ClientTypeVergeRev
		info.IsMeta = true
	case containsAny(ua, "clash verge", "clash-verge", "clashverge"):
		info.Type = model.ClientTypeClashVerge
		info.IsMeta = true
	case containsAny(ua, "clash nyanpasu", "clash-nyanpasu", "nyanpasu"):
		info.Type = model.ClientTypeNyanpasu
		info.IsMeta = true
	case containsAny(ua, "clash for android", "clashforandroid", "cfa", "clash.meta for android"):
		info.Type = model.ClientTypeClashForAndroid
		info.IsMeta = true
	case containsAny(ua, "clashx pro", "clashx-pro"):
		info.Type = model.ClientTypeClashXPro
	case containsAny(ua, "clashx", "clash-x"):
		info.Type = model.ClientTypeClashX
	case containsAny(ua, "clash for windows", "clashforwindows", "cfw"):
		info.Type = model.ClientTypeCFW
	case containsAny(ua, "flclash", "fl-clash"):
		info.Type = model.ClientTypeFlClash
		info.IsMeta = true
	case containsAny(ua, "karing"):
		info.Type = model.ClientTypeKaring
		info.IsMeta = true
	case containsAny(ua, "hiddify-next", "hiddifynext"):
		info.Type = model.ClientTypeHiddifyNext
		info.IsMeta = true
	case containsAny(ua, "hiddify"):
		info.Type = model.ClientTypeHiddify
		info.IsMeta = true
	case containsAny(ua, "clash"):
		info.Type = model.ClientTypeClash
		if strings.Contains(ua, "meta") {
			info.Type = model.ClientTypeClashMeta
			info.IsMeta = true
		}
	case containsAny(ua, "sing-box for apple", "sfi"):
		info.Type = model.ClientTypeSFI
	case containsAny(ua, "sing-box for android", "sfa"):
		info.Type = model.ClientTypeSFA
	case containsAny(ua, "sing-box for macos", "sfm"):
		info.Type = model.ClientTypeSFM
	case containsAny(ua, "sing-box", "singbox", "sing box"):
		info.Type = model.ClientTypeSingBox
	case containsAny(ua, "sub-store", "substore"):
		info.Type = model.ClientTypeSubStore
	case containsAny(ua, "shadowrocket", "shadow rocket", "rocket"):
		info.Type = model.ClientTypeShadowrocket
	case containsAny(ua, "streisand"):
		info.Type = model.ClientTypeStreisand
	case containsAny(ua, "v2box"):
		info.Type = model.ClientTypeV2Box
	case containsAny(ua, "v2rayng", "v2ray ng"):
		info.Type = model.ClientTypeV2RayNG
	case containsAny(ua, "nekoray", "neko ray"):
		info.Type = model.ClientTypeNekoRay
	case containsAny(ua, "nekobox", "neko box"):
		info.Type = model.ClientTypeNekoBox
	case containsAny(ua, "v2rayn", "v2ray n", "v2rayn"):
		info.Type = model.ClientTypeV2RayN
	case containsAny(ua, "surfboard"):
		info.Type = model.ClientTypeSurfboard
	case containsAny(ua, "quantumult%20x", "quantumult x", "quantumultx", "quanx", "quan x"):
		info.Type = model.ClientTypeQuantumultX
	case containsAny(ua, "quantumult", "quan"):
		info.Type = model.ClientTypeQuantumult
	case containsAny(ua, "loon-lite", "loon lite"):
		info.Type = model.ClientTypeLoonLite
	case containsAny(ua, "loon"):
		info.Type = model.ClientTypeLoon
	case containsAny(ua, "stash"):
		info.Type = model.ClientTypeStash
	case containsAny(ua, "momo"):
		info.Type = model.ClientTypeMomo
	case containsAny(ua, "surge"):
		info.Type = model.ClientTypeSurge
	case containsAny(ua, "xbrowser", "x browser"):
		info.Type = model.ClientTypeXBrowser
	}

	return info
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func ClientToRenderer(ct model.ClientType) string {
	switch ct {
	case model.ClientTypeClashMeta, model.ClientTypeMihomo, model.ClientTypeMihomoParty,
		model.ClientTypeClashVerge, model.ClientTypeVergeRev, model.ClientTypeNyanpasu,
		model.ClientTypeClashForAndroid, model.ClientTypeFlClash, model.ClientTypeKaring,
		model.ClientTypeHiddify, model.ClientTypeHiddifyNext:
		return "clashmeta"
	case model.ClientTypeClash, model.ClientTypeClashX, model.ClientTypeClashXPro,
		model.ClientTypeCFW:
		return "clash"
	case model.ClientTypeSingBox, model.ClientTypeSFA, model.ClientTypeSFI, model.ClientTypeSFM:
		return "singbox"
	case model.ClientTypeSurge, model.ClientTypeSurgeMac, model.ClientTypeSurgeiOS:
		return "surge"
	case model.ClientTypeStash:
		return "clashmeta"
	case model.ClientTypeShadowrocket, model.ClientTypeV2RayN, model.ClientTypeV2RayNG,
		model.ClientTypeNekoBox, model.ClientTypeNekoRay, model.ClientTypeV2Box,
		model.ClientTypeQuantumult, model.ClientTypeQuantumultX,
		model.ClientTypeLoon, model.ClientTypeLoonLite, model.ClientTypeSurfboard,
		model.ClientTypeStreisand, model.ClientTypeMomo, model.ClientTypeSubStore,
		model.ClientTypeXBrowser, model.ClientTypeBox:
		return "uri"
	default:
		return "uri"
	}
}

func NormalizeClientType(ct string) model.ClientType {
	normalized := strings.ToLower(strings.TrimSpace(ct))
	switch {
	case normalized == "clash":
		return model.ClientTypeClash
	case normalized == "clash-meta" || normalized == "clashmeta" || normalized == "mihomo" || normalized == "clash.meta":
		return model.ClientTypeClashMeta
	case normalized == "mihomo-party":
		return model.ClientTypeMihomoParty
	case normalized == "clash-verge" || normalized == "clashverge":
		return model.ClientTypeClashVerge
	case normalized == "verge-rev" || normalized == "verge rev":
		return model.ClientTypeVergeRev
	case normalized == "clash-nyanpasu" || normalized == "nyanpasu":
		return model.ClientTypeNyanpasu
	case normalized == "clash-for-android" || normalized == "clashforandroid" || normalized == "cfa":
		return model.ClientTypeClashForAndroid
	case normalized == "clashx" || normalized == "clash-x":
		return model.ClientTypeClashX
	case normalized == "clashx-pro" || normalized == "clashx pro":
		return model.ClientTypeClashXPro
	case normalized == "clash-for-windows" || normalized == "clashforwindows" || normalized == "cfw":
		return model.ClientTypeCFW
	case normalized == "flclash":
		return model.ClientTypeFlClash
	case normalized == "karing":
		return model.ClientTypeKaring
	case normalized == "hiddify-next" || normalized == "hiddifynext":
		return model.ClientTypeHiddifyNext
	case normalized == "hiddify":
		return model.ClientTypeHiddify
	case normalized == "sing-box" || normalized == "singbox":
		return model.ClientTypeSingBox
	case normalized == "sfa" || normalized == "sing-box-for-android":
		return model.ClientTypeSFA
	case normalized == "sfi" || normalized == "sing-box-for-apple":
		return model.ClientTypeSFI
	case normalized == "sfm" || normalized == "sing-box-for-macos":
		return model.ClientTypeSFM
	case normalized == "surge":
		return model.ClientTypeSurge
	case normalized == "quantumult" || normalized == "quan":
		return model.ClientTypeQuantumult
	case normalized == "quanx" || normalized == "quantumultx" || normalized == "quantumult-x" || normalized == "quantumult x":
		return model.ClientTypeQuantumultX
	case normalized == "shadowrocket":
		return model.ClientTypeShadowrocket
	case normalized == "v2rayn":
		return model.ClientTypeV2RayN
	case normalized == "v2rayng":
		return model.ClientTypeV2RayNG
	case normalized == "nekobox":
		return model.ClientTypeNekoBox
	case normalized == "nekoray":
		return model.ClientTypeNekoRay
	case normalized == "v2box":
		return model.ClientTypeV2Box
	case normalized == "stash":
		return model.ClientTypeStash
	case normalized == "loon":
		return model.ClientTypeLoon
	case normalized == "loon-lite":
		return model.ClientTypeLoonLite
	case normalized == "surfboard":
		return model.ClientTypeSurfboard
	case normalized == "streisand":
		return model.ClientTypeStreisand
	case normalized == "momo":
		return model.ClientTypeMomo
	case normalized == "sub-store" || normalized == "substore":
		return model.ClientTypeSubStore
	case normalized == "xbrowser":
		return model.ClientTypeXBrowser
	case normalized == "" || normalized == "uri" || normalized == "base64":
		return model.ClientTypeURI
	default:
		return model.ClientTypeURI
	}
}
