package renderer

import (
	"fmt"

	"github.com/airport-panel/subscription"
	subrend "github.com/airport-panel/subscription/renderer"
)

var registry *subscription.Registry

func InitRendererRegistry() {
	registry = subscription.NewRegistry()
	registry.Register(subrend.NewClashRenderer())
	registry.Register(subrend.NewClashMetaRenderer())
	registry.Register(subrend.NewSingBoxRenderer())
	registry.Register(subrend.NewURIRenderer())
}

func GetRenderer(name string) (subscription.Renderer, error) {
	if registry == nil {
		InitRendererRegistry()
	}
	r := registry.Get(name)
	if r == nil {
		return nil, fmt.Errorf("renderer not found: %s", name)
	}
	return r, nil
}

func DefaultName() string { return "uri" }

func ListRendererNames() []string {
	if registry == nil {
		InitRendererRegistry()
	}
	return registry.Names()
}
