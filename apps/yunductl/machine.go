// machine.go 实现 yunductl machine list 子命令。
//
// 该命令远程查询面板 API（GET /api/v1/agent/machine/nodes）获取该机器上
// 所有节点的列表，用于 Machine 模式下查看当前 VPS 托管了哪些节点。
//
// 与 node 模式的区别：
//   - yunductl status/nodes：通过 Unix Socket 查询本地 agent 状态
//   - yunductl machine list：通过 HTTP 查询面板 API（因为单个 agent 可能托管多个节点）
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// machineNodeEntry 与 node-agent/cmd/agent/machine.go 中的结构保持一致。
type machineNodeEntry struct {
	ServerCode  string `json:"server_code"`
	RuntimeType string `json:"runtime_type"`
	RuntimeID   string `json:"runtime_id"`
}

type machineNodesResponse struct {
	Nodes []machineNodeEntry `json:"nodes"`
}

// listMachineNodes 远程查询面板 API 获取 Machine 模式节点列表。
//
// 环境变量：
//   - YUNDU_MACHINE_PANEL_URL：面板 URL（如 https://panel.example.com）
//   - YUNDU_MACHINE_TOKEN：server_token，用于面板认证
func listMachineNodes() {
	panelURL := os.Getenv("YUNDU_MACHINE_PANEL_URL")
	if panelURL == "" {
		fmt.Fprintf(os.Stderr, "错误: 未设置 YUNDU_MACHINE_PANEL_URL 环境变量\n")
		fmt.Fprintf(os.Stderr, "示例: YUNDU_MACHINE_PANEL_URL=https://panel.example.com yunductl machine list\n")
		os.Exit(1)
	}
	token := os.Getenv("YUNDU_MACHINE_TOKEN")
	if token == "" {
		fmt.Fprintf(os.Stderr, "错误: 未设置 YUNDU_MACHINE_TOKEN 环境变量\n")
		os.Exit(1)
	}

	url := fmt.Sprintf("%s/api/v1/agent/machine/nodes?server_token=%s", panelURL, token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "查询面板 API 失败: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取响应失败: %v\n", err)
		os.Exit(1)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "面板返回错误 (HTTP %d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var result machineNodesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		// 面板 API 响应格式可能不同，直接打印原始 JSON
		fmt.Println(prettyJSON(body))
		return
	}

	if len(result.Nodes) == 0 {
		fmt.Println("该机器上没有托管的节点")
		return
	}

	fmt.Printf("共 %d 个节点:\n\n", len(result.Nodes))
	for i, n := range result.Nodes {
		fmt.Printf("  [%d] server_code=%s  runtime=%s  runtime_id=%s\n",
			i+1, n.ServerCode, n.RuntimeType, n.RuntimeID)
	}
}
