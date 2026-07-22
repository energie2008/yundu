package renderer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/airport-panel/subscription"
	"github.com/airport-panel/subscription-service/internal/client"
	"github.com/airport-panel/subscription-service/internal/model"
	"github.com/airport-panel/subscription/nodespec"
	subrend "github.com/airport-panel/subscription/renderer"
	"gopkg.in/yaml.v3"
)

func init() {
	InitRendererRegistry()
}

type SubscriptionRenderer struct {
	rendererName string
	renderer     subscription.Renderer
	clientType   model.ClientType
	baseTemplate string // 来自数据库的订阅模板内容（为空时使用渲染器内置默认模板）
}

func NewSubscriptionRenderer(ct model.ClientType) *SubscriptionRenderer {
	rName := client.ClientToRenderer(ct)
	r, _ := GetRenderer(rName)
	if r == nil {
		r = subrend.NewURIRenderer()
		rName = "uri"
	}
	return &SubscriptionRenderer{
		rendererName: rName,
		renderer:     r,
		clientType:   ct,
	}
}

// WithBaseTemplate 设置来自数据库的订阅模板内容。
// 设置后，Render 会以该模板为基础，将渲染出的节点信息填入模板的
// proxies（Clash）或 outbounds（SingBox）字段，保留模板中的
// proxy-groups / rules / dns / route 等自定义配置。
// 如果模板解析失败，自动回退到渲染器内置默认模板。
func (sr *SubscriptionRenderer) WithBaseTemplate(tmpl string) *SubscriptionRenderer {
	sr.baseTemplate = tmpl
	return sr
}

func (sr *SubscriptionRenderer) ContentType() string {
	if sr.renderer == nil {
		return "text/plain; charset=utf-8"
	}
	return sr.renderer.ContentType()
}

func (sr *SubscriptionRenderer) Render(nodes []*model.NodeInfo, rc *model.RenderContext) (string, error) {
	specs := NodeInfosToNodeSpecs(nodes)
	if sr.renderer == nil {
		return "", fmt.Errorf("no renderer")
	}

	var content string
	if sr.baseTemplate != "" {
		// 使用数据库模板作为基础，填入节点信息
		rendered, err := sr.renderWithTemplate(specs)
		if err != nil {
			// 模板渲染失败，回退到内置默认渲染器
			data, derr := sr.renderer.Render(specs)
			if derr != nil {
				return "", derr
			}
			content = string(data)
		} else {
			content = rendered
		}
	} else {
		data, err := sr.renderer.Render(specs)
		if err != nil {
			return "", err
		}
		content = string(data)
	}

	if sr.rendererName == "uri" {
		lines := strings.Split(strings.TrimSpace(content), "\n")
		var filtered []string
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" {
				filtered = append(filtered, l)
			}
		}
		joined := strings.Join(filtered, "\n")
		content = base64.StdEncoding.EncodeToString([]byte(joined))
	}

	return content, nil
}

// renderWithTemplate 使用数据库模板作为基础配置，将渲染出的节点信息注入模板。
// - Clash/ClashMeta（YAML）：将节点 proxies 填入模板的 proxies 字段，
//   并为 proxies 为空的 proxy-group 自动填充节点名称。
// - SingBox（JSON）：将节点 outbounds 追加到模板的 outbounds 列表，
//   并为 outbounds 为空的 urltest/selector 自动填充节点 tag。
// - 其他渲染器（URI/Surge 等）：不支持模板，回退到内置默认渲染。
func (sr *SubscriptionRenderer) renderWithTemplate(specs []nodespec.NodeSpec) (string, error) {
	switch sr.rendererName {
	case "clash", "clashmeta":
		return sr.renderClashWithTemplate(specs)
	case "singbox":
		return sr.renderSingBoxWithTemplate(specs)
	default:
		data, err := sr.renderer.Render(specs)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}

// renderClashWithTemplate 将节点信息注入 Clash YAML 模板。
func (sr *SubscriptionRenderer) renderClashWithTemplate(specs []nodespec.NodeSpec) (string, error) {
	var tmpl map[string]interface{}
	if err := yaml.Unmarshal([]byte(sr.baseTemplate), &tmpl); err != nil {
		return "", fmt.Errorf("parse clash template: %w", err)
	}

	// 渲染每个节点的 proxy 定义
	proxies := make([]interface{}, 0, len(specs))
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		nodeYAML, err := sr.renderer.RenderNode(spec)
		if err != nil {
			continue
		}
		var proxy map[string]interface{}
		if err := yaml.Unmarshal([]byte(nodeYAML), &proxy); err == nil {
			proxies = append(proxies, proxy)
			if name, ok := proxy["name"].(string); ok && name != "" {
				names = append(names, name)
			}
		}
	}
	tmpl["proxies"] = proxies

	// 为 proxies 列表为空的 proxy-group 自动填充所有节点名称
	if groups, ok := tmpl["proxy-groups"].([]interface{}); ok {
		for i, g := range groups {
			if gm, ok := g.(map[string]interface{}); ok {
				if existing, ok := gm["proxies"].([]interface{}); ok && len(existing) == 0 && len(names) > 0 {
					filled := make([]interface{}, len(names))
					for j, n := range names {
						filled[j] = n
					}
					gm["proxies"] = filled
					groups[i] = gm
				}
			}
		}
		tmpl["proxy-groups"] = groups
	}

	out, err := yaml.Marshal(tmpl)
	if err != nil {
		return "", fmt.Errorf("marshal clash template: %w", err)
	}
	return string(out), nil
}

// renderSingBoxWithTemplate 将节点信息注入 SingBox JSON 模板。
func (sr *SubscriptionRenderer) renderSingBoxWithTemplate(specs []nodespec.NodeSpec) (string, error) {
	var tmpl map[string]interface{}
	if err := json.Unmarshal([]byte(sr.baseTemplate), &tmpl); err != nil {
		return "", fmt.Errorf("parse singbox template: %w", err)
	}

	// 渲染每个节点的 outbound 定义
	nodeOutbounds := make([]interface{}, 0, len(specs))
	tags := make([]string, 0, len(specs))
	for _, spec := range specs {
		nodeJSON, err := sr.renderer.RenderNode(spec)
		if err != nil {
			continue
		}
		var ob map[string]interface{}
		if err := json.Unmarshal([]byte(nodeJSON), &ob); err == nil {
			nodeOutbounds = append(nodeOutbounds, ob)
			if tag, ok := ob["tag"].(string); ok && tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	// 将节点 outbounds 追加到模板已有的 outbounds 列表末尾
	existingOuts, _ := tmpl["outbounds"].([]interface{})
	merged := make([]interface{}, 0, len(existingOuts)+len(nodeOutbounds))
	merged = append(merged, existingOuts...)
	merged = append(merged, nodeOutbounds...)

	// 为 outbounds 列表为空的 urltest/selector 自动填充所有节点 tag
	for i, ob := range merged {
		if om, ok := ob.(map[string]interface{}); ok {
			t, _ := om["type"].(string)
			if t == "urltest" || t == "selector" {
				if outs, ok := om["outbounds"].([]interface{}); ok && len(outs) == 0 && len(tags) > 0 {
					filled := make([]interface{}, len(tags))
					for j, tag := range tags {
						filled[j] = tag
					}
					om["outbounds"] = filled
					merged[i] = om
				}
			}
		}
	}
	tmpl["outbounds"] = merged

	out, err := json.MarshalIndent(tmpl, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal singbox template: %w", err)
	}
	return string(out), nil
}

func (sr *SubscriptionRenderer) RenderName() string {
	return sr.rendererName
}
