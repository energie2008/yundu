package service

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

// DeviceStateService 管理跨节点设备状态聚合。
//
// 参考 Xboard 的 DeviceStateService.php，使用 Redis Hash 存储每个用户的在线设备态：
//
//	key   = yundu:user_devices:{userId}   (Hash)
//	field = {nodeId}:{ip}                 (节点 + 归一化 IP 联合主键，同一 IP 在不同节点各占一条)
//	value = unix 秒级时间戳               (最后一次上报时间)
//
// TTL: 300s (5 分钟)。与 repo.OnlineTTL 保持一致。
//
// 读取时额外剔除超过 staleTTL 的过期记录，避免节点停止上报后残留记录干扰设备数统计。
type DeviceStateService struct {
	redisClient *goredis.Client
	logger      *slog.Logger
}

const (
	// deviceKeyPrefix Redis Hash key 前缀：yundu:user_devices:{userId}
	deviceKeyPrefix = "yundu:user_devices:"
	// deviceTTL 设备态记录的过期时间（5 分钟），与 repo.OnlineTTL 对齐。
	deviceTTL = 300 * time.Second
	// staleTTL 读取时判定记录是否过期的阈值。
	// 超过该时长未刷新的设备记录视为离线（退化为不计入设备数）。
	staleTTL = 300 * time.Second
	// scanCount SCAN 命令的 COUNT 提示值。
	scanCount = 200
)

// NewDeviceStateService 创建设备状态服务。
// redisClient 为 nil 时所有写操作为空操作、读操作返回空结果，便于无 Redis 环境降级。
// logger 为 nil 时回退到 slog.Default()。
func NewDeviceStateService(redisClient *goredis.Client, logger *slog.Logger) *DeviceStateService {
	if logger == nil {
		logger = slog.Default()
	}
	return &DeviceStateService{
		redisClient: redisClient,
		logger:      logger,
	}
}

// deviceKey 构造某用户的 Redis Hash key。
func (s *DeviceStateService) deviceKey(userID uuid.UUID) string {
	return deviceKeyPrefix + userID.String()
}

// SetDevices Node 上报在线 IP。
//
// 将该用户在本节点的所有在线 IP 写入其对应的 Redis Hash：
//   - field = {nodeId}:{归一化IP}
//   - value = 当前 unix 时间戳
//
// 同时刷新该 Hash 的 TTL。同一上报内对重复 IP 自动去重。
// 详见 Xboard DeviceStateService::setDevices。
func (s *DeviceStateService) SetDevices(ctx context.Context, userID uuid.UUID, nodeID uuid.UUID, ips []string) error {
	if s.redisClient == nil {
		return nil
	}
	if len(ips) == 0 {
		return nil
	}

	key := s.deviceKey(userID)
	nodeIDStr := nodeID.String()
	now := time.Now().Unix()

	// 构造 HSet 参数：field1, value1, field2, value2, ...
	// 同一上报内对归一化后的 IP 去重，避免重复 field。
	seen := make(map[string]struct{}, len(ips))
	values := make([]interface{}, 0, len(ips)*2)
	for _, ip := range ips {
		clean := s.NormalizeIP(ip)
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		field := nodeIDStr + ":" + clean
		values = append(values, field, now)
	}
	if len(values) == 0 {
		return nil
	}

	// 使用 pipeline 原子地写入并刷新 TTL，减少往返。
	pipe := s.redisClient.Pipeline()
	pipe.HSet(ctx, key, values...)
	pipe.Expire(ctx, key, deviceTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("set devices for user %s on node %s: %w", userID, nodeID, err)
	}
	return nil
}

// GetAliveList 获取各用户当前在线设备数。
//
// 遍历各用户的 Hash，剔除超过 staleTTL 的过期记录后统计有效设备数。
// 返回 map[userID]int。未查询到记录的用户返回 0。
func (s *DeviceStateService) GetAliveList(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	result := make(map[uuid.UUID]int, len(userIDs))
	if s.redisClient == nil {
		return result, nil
	}
	if len(userIDs) == 0 {
		return result, nil
	}

	cutoff := time.Now().Add(-staleTTL).Unix()
	for _, userID := range userIDs {
		key := s.deviceKey(userID)
		fields, err := s.redisClient.HGetAll(ctx, key).Result()
		if err != nil {
			return nil, fmt.Errorf("get devices for user %s: %w", userID, err)
		}
		count := 0
		for _, tsStr := range fields {
			ts, err := strconv.ParseInt(tsStr, 10, 64)
			if err != nil {
				continue
			}
			if ts >= cutoff {
				count++
			}
		}
		result[userID] = count
	}
	return result, nil
}

// GetUsersDevices 获取多用户设备列表（返回去重后的 IP 列表）。
//
// 遍历各用户的 Hash，剔除过期记录后，从 field={nodeId}:{ip} 中提取 IP 并去重。
// 返回 map[userID][]string。未查询到记录的用户返回 nil 切片。
func (s *DeviceStateService) GetUsersDevices(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	result := make(map[uuid.UUID][]string, len(userIDs))
	if s.redisClient == nil {
		return result, nil
	}
	if len(userIDs) == 0 {
		return result, nil
	}

	cutoff := time.Now().Add(-staleTTL).Unix()
	for _, userID := range userIDs {
		key := s.deviceKey(userID)
		fields, err := s.redisClient.HGetAll(ctx, key).Result()
		if err != nil {
			return nil, fmt.Errorf("get devices for user %s: %w", userID, err)
		}
		seen := make(map[string]struct{}, len(fields))
		var ips []string
		for field, tsStr := range fields {
			ts, err := strconv.ParseInt(tsStr, 10, 64)
			if err != nil {
				continue
			}
			if ts < cutoff {
				continue
			}
			// field = {nodeId}:{ip}，提取首个冒号之后的 IP 部分
			// 注意：IPv6 地址本身包含冒号，因此只能按第一个冒号切分。
			ip := extractIPFromField(field)
			if ip == "" {
				continue
			}
			if _, ok := seen[ip]; ok {
				continue
			}
			seen[ip] = struct{}{}
			ips = append(ips, ip)
		}
		result[userID] = ips
	}
	return result, nil
}

// ClearAllNodeDevices 节点断连时清除该节点所有设备记录。
//
// 由于设备态按 user 维度分桶存储在多个 Hash 中，无法直接按 node 定位，
// 因此先 SCAN 所有 yundu:user_devices:* key，再逐个 Hash 删除以 {nodeId}: 为前缀的 field。
// 返回受影响（实际删除了至少一条 field）的 user key 数量。
func (s *DeviceStateService) ClearAllNodeDevices(ctx context.Context, nodeID uuid.UUID) error {
	affected, err := s.ClearAllNodeDevicesCounted(ctx, nodeID)
	if err != nil {
		return err
	}
	s.logger.Info("cleared node devices", "node_id", nodeID, "affected_users", affected)
	return nil
}

// ClearAllNodeDevicesCounted 与 ClearAllNodeDevices 相同，但额外返回受影响的 user key 数量。
// 供 handler 在响应中回显清理规模。
func (s *DeviceStateService) ClearAllNodeDevicesCounted(ctx context.Context, nodeID uuid.UUID) (int, error) {
	if s.redisClient == nil {
		return 0, nil
	}

	nodePrefix := nodeID.String() + ":"
	keyPattern := deviceKeyPrefix + "*"
	cleared := 0

	iter := s.redisClient.Scan(ctx, 0, keyPattern, scanCount).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		fields, err := s.redisClient.HKeys(ctx, key).Result()
		if err != nil {
			s.logger.Warn("clear node devices: HKeys failed, skipping key",
				"key", key, "node_id", nodeID, "error", err)
			continue
		}
		var toDelete []string
		for _, f := range fields {
			if strings.HasPrefix(f, nodePrefix) {
				toDelete = append(toDelete, f)
			}
		}
		if len(toDelete) == 0 {
			continue
		}
		if err := s.redisClient.HDel(ctx, key, toDelete...).Err(); err != nil {
			s.logger.Warn("clear node devices: HDel failed, skipping key",
				"key", key, "node_id", nodeID, "count", len(toDelete), "error", err)
			continue
		}
		cleared++
	}
	if err := iter.Err(); err != nil {
		return cleared, fmt.Errorf("clear node devices scan for node %s: %w", nodeID, err)
	}
	return cleared, nil
}

// NormalizeIP 剥离端口后缀，返回纯 IP。
//
// 支持以下格式：
//   - IPv4:port      -> IPv4
//   - [IPv6]:port    -> IPv6
//   - 纯 IPv4/IPv6   -> 原样返回
//
// 使用 net.SplitHostPort 统一处理带端口的 host:port 与 [host]:port 形式；
// 解析失败（无端口或纯 IPv6 无方括号）时原样返回输入。
func (s *DeviceStateService) NormalizeIP(ip string) string {
	if ip == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(ip); err == nil {
		return host
	}
	return ip
}

// extractIPFromField 从 "{nodeId}:{ip}" field 中提取 IP 部分。
//
// 仅按首个冒号切分，以正确处理 IPv6 地址（其本身包含多个冒号）。
func extractIPFromField(field string) string {
	idx := strings.Index(field, ":")
	if idx < 0 {
		return ""
	}
	return field[idx+1:]
}
