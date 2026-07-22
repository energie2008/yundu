// yunductl 是 YunDu Agent 的本地控制 CLI 工具。
//
// Node 模式：通过 Unix Socket（/run/yundu/agent.sock）与运行中的 node-agent 通信。
// Machine 模式：通过 HTTP API（:10000）与 MachineOrchestrator 通信，支持按 server_code 路由。
//
// 执行状态查询、配置刷新、回滚、重启、诊断等操作，无需 SSH 进入节点。
//
// 用法：
//
//	yunductl <command> [flags]
//
// 子命令：
//
//	status        查询节点状态快照（版本/配置版本/运行时状态/通道）
//	refresh       强制从面板拉取最新配置
//	rollback      回滚到 LKG（Last Known Good）配置
//	restart       重启内核（不重启 agent 进程）
//	diag          诊断信息（xray gRPC 可达性/配置目录/版本）
//	nodes         节点列表（node 模式返回单节点，machine 模式返回所有节点）
//	logs          查看节点日志（支持 --node 指定节点）
//	bind          查看端口绑定信息（支持 --node 指定节点）
//	upgrade       触发自升级检查
//	machine list  Machine 模式节点列表（远程查询 panel API）
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ControlSocketPath = "/run/yundu/agent.sock"

type transportMode int

const (
	modeUnix transportMode = iota
	modeHTTP
)

var (
	httpAddr   string
	nodeFilter string
)

func init() {
	flag.StringVar(&httpAddr, "http", "", "Machine模式HTTP地址 (如 http://127.0.0.1:10000)")
	flag.StringVar(&nodeFilter, "node", "", "指定节点 server_code (machine模式)")
}

func currentTransportMode() transportMode {
	if httpAddr != "" {
		return modeHTTP
	}
	if env := os.Getenv("YUNDU_HTTP_ADDR"); env != "" {
		httpAddr = env
		return modeHTTP
	}
	return modeUnix
}

func newClient() *http.Client {
	if currentTransportMode() == modeHTTP {
		return &http.Client{Timeout: 30 * time.Second}
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", ControlSocketPath)
			},
		},
	}
}

func buildURL(path string) string {
	if currentTransportMode() == modeHTTP {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		if nodeFilter != "" {
			return fmt.Sprintf("%s%s%sserver_code=%s", httpAddr, path, sep, nodeFilter)
		}
		return httpAddr + path
	}
	return "http://unix" + path
}

func sendRequest(method, path string) ([]byte, error) {
	client := newClient()
	url := buildURL(path)

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	if nodeFilter != "" && currentTransportMode() == modeHTTP {
		req.Header.Set("X-Server-Code", nodeFilter)
	}

	resp, err := client.Do(req)
	if err != nil {
		if currentTransportMode() == modeUnix {
			return nil, fmt.Errorf("连接 agent 失败 (socket: %s): %w", ControlSocketPath, err)
		}
		return nil, fmt.Errorf("连接 orchestrator 失败 (%s): %w", httpAddr, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return body, fmt.Errorf("agent 返回错误 (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return body, nil
}

func prettyJSON(raw []byte) string {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(pretty)
}

func printResult(raw []byte) {
	fmt.Println(prettyJSON(raw))
}

func printLogs(logDir string, lines int) {
	if logDir == "" {
		fmt.Fprintln(os.Stderr, "错误: 无法确定日志目录")
		return
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取日志目录失败: %v\n", err)
		return
	}
	var logFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
			logFiles = append(logFiles, filepath.Join(logDir, e.Name()))
		}
	}
	if len(logFiles) == 0 {
		fmt.Println("未找到日志文件")
		return
	}
	latest := logFiles[len(logFiles)-1]
	data, err := os.ReadFile(latest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取日志文件失败: %v\n", err)
		return
	}
	lines_all := strings.Split(string(data), "\n")
	start := 0
	if len(lines_all) > lines {
		start = len(lines_all) - lines
	}
	for i := start; i < len(lines_all); i++ {
		fmt.Println(lines_all[i])
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `yunductl - YunDu Agent 本地控制工具

用法:
  yunductl [flags] <command> [flags]

Flags:
  --http <addr>    Machine模式使用HTTP地址 (如 http://127.0.0.1:10000)
  --node <code>    指定节点 server_code (machine模式)

子命令:
  status        查询节点状态快照
  refresh       强制从面板拉取最新配置
  rollback      回滚到 LKG 配置
  restart       重启内核（不重启 agent）
  diag          诊断信息（xray gRPC 可达性等）
  nodes         节点列表
  logs          查看节点最近日志 (--lines N, 默认100行)
  bind          查看端口绑定信息
  upgrade       触发自升级检查
  machine list  Machine 模式：远程查询 panel 节点列表
  help          显示此帮助信息

环境变量:
  YUNDU_HTTP_ADDR          Machine模式HTTP地址（同--http）
  YUNDU_MACHINE_PANEL_URL  Machine模式查询的面板URL
  YUNDU_MACHINE_TOKEN      Machine模式认证的server_token

示例:
  yunductl status
  yunductl nodes
  yunductl --http http://127.0.0.1:10000 nodes
  yunductl --http http://127.0.0.1:10000 --node SERVER_CODE status
  yunductl logs --lines 50

`)
}

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		usage()
		os.Exit(1)
	}

	cmd := args[0]

	switch cmd {
	case "status":
		body, err := sendRequest("GET", "/v1/status")
		if err != nil {
			body, err = sendRequest("GET", "/status")
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		printResult(body)

	case "refresh":
		body, err := sendRequest("POST", "/v1/refresh")
		if err != nil {
			body, err = sendRequest("POST", "/refresh")
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		printResult(body)

	case "rollback":
		body, err := sendRequest("POST", "/rollback")
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		printResult(body)

	case "restart":
		body, err := sendRequest("POST", "/v1/restart")
		if err != nil {
			body, err = sendRequest("POST", "/restart")
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		printResult(body)

	case "diag":
		body, err := sendRequest("GET", "/v1/diag")
		if err != nil {
			body, err = sendRequest("GET", "/diag")
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		printResult(body)

	case "nodes":
		body, err := sendRequest("GET", "/nodes")
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		printResult(body)

	case "logs":
		logCmd := flag.NewFlagSet("logs", flag.ExitOnError)
		logLines := logCmd.Int("lines", 100, "显示最近N行日志")
		logCmd.Parse(args[1:])
		_ = logLines
		if currentTransportMode() == modeHTTP {
			body, err := sendRequest("GET", "/nodes")
			if err != nil {
				fmt.Fprintf(os.Stderr, "错误: %v\n", err)
				os.Exit(1)
			}
			var nodeList struct {
				Nodes []struct {
					ServerCode string `json:"server_code"`
					ConfigDir  string `json:"config_dir"`
				} `json:"nodes"`
			}
			json.Unmarshal(body, &nodeList)
			for _, n := range nodeList.Nodes {
				if nodeFilter != "" && n.ServerCode != nodeFilter {
					continue
				}
				logDir := filepath.Join(filepath.Dir(filepath.Dir(n.ConfigDir)), "logs", "nodes", n.ServerCode)
				fmt.Printf("=== 节点 %s 日志 ===\n", n.ServerCode)
				printLogs(logDir, *logLines)
			}
		} else {
			defaultLogDir := "/var/log/yundu"
			if d := os.Getenv("YUNDU_LOG_DIR"); d != "" {
				defaultLogDir = d
			}
			printLogs(defaultLogDir, *logLines)
		}

	case "bind":
		body, err := sendRequest("GET", "/diag")
		if err != nil {
			body, err = sendRequest("GET", "/v1/diag")
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		printResult(body)

	case "upgrade":
		body, err := sendRequest("POST", "/upgrade")
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		printResult(body)

	case "machine":
		if len(args) < 2 || args[1] != "list" {
			fmt.Fprintf(os.Stderr, "用法: yunductl machine list\n")
			os.Exit(1)
		}
		listMachineNodes()

	case "help", "-h", "--help":
		usage()

	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", cmd)
		usage()
		os.Exit(1)
	}
}
