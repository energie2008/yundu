package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	statsCmd "github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// 诊断工具：直接查询 xray StatsService gRPC API，打印所有统计数据。
// 用法: ./statsprobe [endpoint]
// 默认 endpoint: 127.0.0.1:10085
func main() {
	endpoint := "127.0.0.1:10085"
	if len(os.Args) > 1 {
		endpoint = os.Args[1]
	}

	fmt.Printf("Connecting to xray StatsService at %s ...\n", endpoint)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		fmt.Printf("ERROR: failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	fmt.Println("Connected. Querying stats (Reset=false) ...")

	client := statsCmd.NewStatsServiceClient(conn)

	// Query WITHOUT reset to see accumulated stats
	resp, err := client.QueryStats(ctx, &statsCmd.QueryStatsRequest{Reset_: false})
	if err != nil {
		fmt.Printf("ERROR: QueryStats failed: %v\n", err)
		os.Exit(1)
	}

	stats := resp.GetStat()
	fmt.Printf("Total stats returned: %d\n\n", len(stats))

	if len(stats) == 0 {
		fmt.Println("=== NO STATS FOUND ===")
		fmt.Println("This means xray StatsService is not recording any traffic data.")
		fmt.Println("Possible causes:")
		fmt.Println("  1. policy.levels.0.statsUserUplink/Downlink not enabled")
		fmt.Println("  2. Users don't have email field set in inbound config")
		fmt.Println("  3. Stats feature not properly initialized in native mode")
		os.Exit(0)
	}

	for i, stat := range stats {
		fmt.Printf("[%d] name=%q value=%d\n", i, stat.GetName(), stat.GetValue())
	}

	// Also print as JSON for parsing
	fmt.Println("\n=== JSON output ===")
	data, _ := json.MarshalIndent(stats, "", "  ")
	fmt.Println(string(data))
}
