package tcshape

import (
	"context"
	"reflect"
	"sort"
	"testing"
)

// fakeRunner 记录所有命令，不真正执行。
type fakeRunner struct {
	calls [][]string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, append([]string{name}, args...))
	for _, a := range args {
		if a == "-C" {
			// 模拟 iptables 规则不存在：-C 检查失败，随后会执行 -A
			return errRuleNotExist
		}
	}
	return nil
}

type errType string

func (e errType) Error() string { return string(e) }

const errRuleNotExist = errType("rule does not exist")

func TestPortChunks(t *testing.T) {
	got := portChunks([]int{1, 2, 3, 4}, 2)
	want := [][]int{{1, 2}, {3, 4}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("portChunks = %v, want %v", got, want)
	}
	if got := portChunks(nil, 2); got != nil {
		t.Fatalf("portChunks(nil) = %v, want nil", got)
	}
}

func TestMergePorts(t *testing.T) {
	got := mergePorts([]int{5, 3}, []int{3, 7})
	want := []int{3, 5, 7}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergePorts = %v, want %v", got, want)
	}
}

func TestSameRule(t *testing.T) {
	a := Rule{ID: 1, IP: "1.2.3.4", UpKbps: 100, DownKbps: 200, Ports: []int{443}}
	b := Rule{ID: 2, IP: "1.2.3.4", UpKbps: 100, DownKbps: 200, Ports: []int{443}}
	if !sameRule(a, b) {
		t.Fatal("rules with same content should be equal")
	}
	b.Ports = []int{80}
	if sameRule(a, b) {
		t.Fatal("rules with different ports should differ")
	}
}

func TestApplyCreatesClassAndMarkRules(t *testing.T) {
	runner := &fakeRunner{}
	m, err := NewManager(Options{
		Dev:      "eth0",
		Ifb:      "ifb0",
		ServerIP: "10.0.0.1",
		StateDir: t.TempDir(),
		Runner:   runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := m.Apply(ctx, Plan{Rules: []Rule{{
		IP: "8.8.8.8", UpKbps: 5000, DownKbps: 10000, Ports: []int{443, 8443},
	}}}); err != nil {
		t.Fatal(err)
	}

	joined := flatten(runner.calls)
	expectContains(t, joined, "tc", "class", "add", "dev", "eth0", "parent", "1:1", "classid", "1:1000", "htb", "rate", "10000kbit")
	expectContains(t, joined, "tc", "class", "add", "dev", "ifb0", "parent", "1:1", "classid", "1:1000", "htb", "rate", "5000kbit")
	expectContains(t, joined, "iptables", "-t", "mangle", "-A", "PREROUTING", "-s", "8.8.8.8", "-p", "tcp", "-m", "multiport", "--dports", "443,8443", "-j", "MARK", "--set-mark", "1000")
	expectContains(t, joined, "iptables", "-t", "mangle", "-A", "OUTPUT", "-d", "8.8.8.8", "-p", "udp", "-m", "multiport", "--sports", "443,8443", "-j", "MARK", "--set-mark", "1000")
}

func TestClearRemovesRules(t *testing.T) {
	runner := &fakeRunner{}
	m, err := NewManager(Options{
		Dev:      "eth0",
		Ifb:      "ifb0",
		ServerIP: "10.0.0.1",
		StateDir: t.TempDir(),
		Runner:   runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := m.Apply(ctx, Plan{Rules: []Rule{{
		IP: "8.8.8.8", UpKbps: 5000, DownKbps: 10000, Ports: []int{443},
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := m.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	joined := flatten(runner.calls)
	expectContains(t, joined, "iptables", "-t", "mangle", "-D", "PREROUTING", "-s", "8.8.8.8", "-p", "tcp")
	expectContains(t, joined, "tc", "class", "del", "dev", "eth0", "classid", "1:1000")
	expectContains(t, joined, "tc", "filter", "del", "dev", "ifb0", "parent", "1:", "prio", "1", "handle", "1000", "fw")
}

func TestDetectRoute(t *testing.T) {
	// 探测函数依赖系统 ip 命令；此处只验证返回格式（无 ip 命令时跳过）。
	dev, ip, err := detectRoute(ExecRunner{})
	if err != nil {
		t.Skipf("ip route unavailable: %v", err)
	}
	if dev == "" || ip == "" {
		t.Fatalf("detectRoute returned empty dev=%q ip=%q", dev, ip)
	}
}

func flatten(rows [][]string) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r...)
	}
	return out
}

func expectContains(t *testing.T, haystack []string, needle ...string) {
	t.Helper()
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if reflect.DeepEqual(haystack[i:i+len(needle)], needle) {
			return
		}
	}
	t.Fatalf("expected command %v not found in %v", needle, haystack)
}

func TestPortsSorted(t *testing.T) {
	ports := []int{443, 80}
	sort.Ints(ports)
	if ports[0] != 80 {
		t.Fatal("sort failed")
	}
}
