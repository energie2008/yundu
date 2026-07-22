package renderer

import (
	"context"
	"log/slog"
)

type CompatFilter interface {
	FilterNodeForClient(ctx context.Context, node map[string]interface{}, clientCode, version string) (bool, string, error)
	RenderWithCompat(ctx context.Context, node map[string]interface{}, clientCode, version string) (map[string]interface{}, []string, error)
}

type NodeRenderer struct {
	compat CompatFilter
	logger *slog.Logger
}

func NewNodeRenderer(compat CompatFilter) *NodeRenderer {
	return &NodeRenderer{compat: compat, logger: slog.Default()}
}

func newRendererWithFilter(compat CompatFilter) *NodeRenderer {
	return &NodeRenderer{compat: compat, logger: slog.Default()}
}

func (r *NodeRenderer) Render(ctx context.Context, node map[string]interface{}, clientCode, version string) (map[string]interface{}, []string) {
	if r == nil || r.compat == nil {
		return node, nil
	}

	ok, reason, err := r.compat.FilterNodeForClient(ctx, node, clientCode, version)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("compat filter error, falling through", "error", err)
		}
		return node, []string{"compat filter error: " + err.Error()}
	}
	if !ok {
		if r.logger != nil {
			r.logger.Debug("node filtered out by compat", "reason", reason)
		}
		return nil, []string{"filtered: " + reason}
	}

	out, warnings, err := r.compat.RenderWithCompat(ctx, node, clientCode, version)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("compat render error, returning original node", "error", err)
		}
		return node, []string{"compat render error: " + err.Error()}
	}
	return out, warnings
}
