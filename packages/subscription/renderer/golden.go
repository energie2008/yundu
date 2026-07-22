package renderer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/airport-panel/subscription/nodespec"
)

var registry = map[string]func() Renderer{}

func init() {
	Register("uri", func() Renderer { return NewURIRenderer() })
	Register("clash", func() Renderer { return NewClashRenderer() })
	Register("singbox", func() Renderer { return NewSingBoxRenderer() })
	Register("quantumult", func() Renderer { return NewQuantumultRenderer() })
	Register("shadowrocket", func() Renderer { return NewShadowrocketRenderer() })
	Register("surge", func() Renderer { return NewSurgeRenderer() })
	Register("loon", func() Renderer { return NewLoonRenderer() })
}

func Register(name string, factory func() Renderer) {
	registry[name] = factory
}

func Get(name string) (Renderer, error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown renderer: %s", name)
	}
	return f(), nil
}

func List() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}

type GoldenCase struct {
	Name     string              `json:"name"`
	Node     nodespec.NodeSpec   `json:"node"`
	Expected map[string]string   `json:"expected"`
}

type GoldenSuite struct {
	Cases []GoldenCase `json:"cases"`
}

func LoadGoldenCases(path string) (*GoldenSuite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var suite GoldenSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return nil, err
	}
	return &suite, nil
}

func RunGoldenTests(t testT, testDataDir string) {
	files, err := filepath.Glob(filepath.Join(testDataDir, "*.json"))
	if err != nil {
		t.Fatalf("failed to list golden files: %v", err)
	}
	for _, f := range files {
		suite, err := LoadGoldenCases(f)
		if err != nil {
			t.Fatalf("load %s: %v", filepath.Base(f), err)
		}
		for _, c := range suite.Cases {
			for fmtName, expected := range c.Expected {
				r, err := Get(fmtName)
				if err != nil {
					t.Errorf("[%s/%s] renderer not found: %v", c.Name, fmtName, err)
					continue
				}
				got, err := r.RenderNode(c.Node)
				if err != nil {
					t.Errorf("[%s/%s] render error: %v", c.Name, fmtName, err)
					continue
				}
				got = normalizeForCompare(got, fmtName)
				expected = normalizeForCompare(expected, fmtName)
				if !reflect.DeepEqual(got, expected) {
					t.Errorf("[%s/%s] mismatch:\n--- expected ---\n%s\n--- got ---\n%s",
						c.Name, fmtName, expected, got)
				}
			}
		}
	}
}

func normalizeForCompare(s, fmtName string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)
	if fmtName == "clash" || fmtName == "surge" || fmtName == "loon" || fmtName == "quantumult" || fmtName == "shadowrocket" {
		lines := strings.Split(s, "\n")
		var filtered []string
		for _, l := range lines {
			trimmed := strings.TrimSpace(l)
			if trimmed == "" {
				continue
			}
			filtered = append(filtered, trimmed)
		}
		s = strings.Join(filtered, "\n")
	}
	return s
}

func supportedFormats(proto nodespec.Protocol) []string {
	all := map[nodespec.Protocol][]string{
		nodespec.ProtocolVLESS:      {"uri", "clash", "singbox", "quantumult", "shadowrocket", "surge", "loon"},
		nodespec.ProtocolVMess:      {"uri", "clash", "singbox", "quantumult", "shadowrocket", "surge", "loon"},
		nodespec.ProtocolTrojan:     {"uri", "clash", "singbox", "quantumult", "shadowrocket", "surge", "loon"},
		nodespec.ProtocolShadowsocks: {"uri", "clash", "singbox", "quantumult", "shadowrocket", "surge", "loon"},
		nodespec.ProtocolHysteria2:  {"uri", "clash", "singbox", "quantumult", "shadowrocket", "surge", "loon"},
		nodespec.ProtocolTUIC:       {"uri", "clash", "singbox", "shadowrocket", "surge"},
	}
	return all[proto]
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v { return true }
	}
	return false
}

func collectAllNodes(suite *GoldenSuite) ([]nodespec.NodeSpec, error) {
	if suite == nil {
		return nil, fmt.Errorf("nil suite")
	}
	var out []nodespec.NodeSpec
	for _, c := range suite.Cases {
		out = append(out, c.Node)
	}
	return out, nil
}

type testT interface {
	Fatalf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

func UpdateGoldenFiles(testDataDir string, nodes []nodespec.NodeSpec, formats []string) error {
	cases := make([]GoldenCase, 0, len(nodes))
	for _, n := range nodes {
		c := GoldenCase{Name: n.Name, Node: n, Expected: map[string]string{}}
		for _, fmtName := range formats {
			r, err := Get(fmtName)
			if err != nil {
				continue
			}
			out, err := r.RenderNode(n)
			if err != nil {
				continue
			}
			c.Expected[fmtName] = strings.TrimSpace(out)
		}
		cases = append(cases, c)
	}
	suite := GoldenSuite{Cases: cases}
	data, _ := json.MarshalIndent(suite, "", "  ")
	outPath := filepath.Join(testDataDir, "nodes.golden.json")
	return os.WriteFile(outPath, data, 0644)
}
