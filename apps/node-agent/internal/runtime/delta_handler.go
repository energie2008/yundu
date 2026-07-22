package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// userEntry 表示当前运行时中的用户状态。
type userEntry struct {
	User
	addedAt time.Time
}

// configMutator 封装对配置 JSON 的增删改操作。
// 通过解析 JSON → 修改 clients 数组 → 重新序列化，实现用户增量更新。
type configMutator struct {
	mu          sync.RWMutex
	configBytes []byte // 当前生效的完整配置
	users       map[string]*userEntry
	logger      *slog.Logger
	version     atomic.Int64
}

// StartFn 是启动新配置的函数签名。
type StartFn func(ctx context.Context, configBytes []byte) error

// newConfigMutator 创建配置变更器。
func newConfigMutator(logger *slog.Logger) *configMutator {
	return &configMutator{
		users:  make(map[string]*userEntry),
		logger: logger,
	}
}

// seed 从初始配置中提取用户基线。
func (m *configMutator) seed(configBytes []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.configBytes = make([]byte, len(configBytes))
	copy(m.configBytes, configBytes)

	users, err := extractUsersFromConfig(configBytes)
	if err != nil {
		m.logger.Warn("delta: extract users from config (non-fatal)", "error", err)
		return nil
	}
	for email, u := range users {
		m.users[email] = &userEntry{User: u, addedAt: time.Now()}
	}
	m.version.Store(1)
	m.logger.Info("delta: seeded baseline", "users", len(m.users))
	return nil
}

// updateUsers 应用增量用户变更并启动新配置。
func (m *configMutator) updateUsers(ctx context.Context, adds []User, dels []string, start StartFn) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. 应用删除
	for _, email := range dels {
		delete(m.users, email)
	}

	// 2. 应用新增/修改
	for _, u := range adds {
		if u.Email == "" {
			continue
		}
		m.users[u.Email] = &userEntry{User: u, addedAt: time.Now()}
	}

	// 3. 将用户变更应用到配置 JSON
	newBytes, err := applyUsersToConfig(m.configBytes, m.users)
	if err != nil {
		return fmt.Errorf("delta: apply users to config: %w", err)
	}

	// 4. 启动新配置
	startTime := time.Now()
	if err := start(ctx, newBytes); err != nil {
		return fmt.Errorf("delta: start new config: %w", err)
	}

	// 5. 更新缓存
	m.configBytes = newBytes
	m.version.Add(1)

	m.logger.Info("delta: users updated",
		"adds", len(adds),
		"dels", len(dels),
		"total", len(m.users),
		"apply_ms", time.Since(startTime).Milliseconds(),
	)
	return nil
}

// userCount 返回当前用户数。
func (m *configMutator) userCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.users)
}

// applyUsersToConfig 将用户映射应用到 Xray/sing-box 配置 JSON。
// 策略：解析 JSON，遍历 inbounds，用最新用户列表替换 clients 数组。
func applyUsersToConfig(configBytes []byte, users map[string]*userEntry) ([]byte, error) {
	// 尝试作为 Xray 配置解析
	var xrayCfg xrayConfigTemplate
	if err := json.Unmarshal(configBytes, &xrayCfg); err == nil && len(xrayCfg.Inbounds) > 0 {
		return applyUsersToXrayConfig(&xrayCfg, users)
	}

	// 尝试作为 sing-box 配置解析
	var sbCfg singboxConfigTemplate
	if err := json.Unmarshal(configBytes, &sbCfg); err == nil && len(sbCfg.Inbounds) > 0 {
		return applyUsersToSingboxConfig(&sbCfg, users)
	}

	return nil, fmt.Errorf("unrecognized config format (neither xray nor sing-box)")
}

// Xray 配置结构（只解析需要修改的部分）。
type xrayConfigTemplate struct {
	Inbounds []xrayInboundTemplate `json:"inbounds"`
	// 其他字段原样保留
	Other json.RawMessage `json:"-"`
}

type xrayInboundTemplate struct {
	Tag         string          `json:"tag,omitempty"`
	Protocol    string          `json:"protocol,omitempty"`
	Port        interface{}     `json:"port,omitempty"`
	Listen      string          `json:"listen,omitempty"`
	Sniffing    json.RawMessage `json:"sniffing,omitempty"`
	Settings    json.RawMessage `json:"settings,omitempty"`
	StreamSettings json.RawMessage `json:"streamSettings,omitempty"`
}

type xraySettingsTemplate struct {
	Clients []json.RawMessage `json:"clients"`
	Other   map[string]json.RawMessage `json:"-"`
}

func (x *xrayConfigTemplate) UnmarshalJSON(data []byte) error {
	// 保留原始 JSON 中的所有字段
	type alias xrayConfigTemplate
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*x = xrayConfigTemplate(a)
	// 保存原始数据以便序列化时保留其他字段
	x.Other = data
	return nil
}

func (x xrayConfigTemplate) MarshalJSON() ([]byte, error) {
	// 重新序列化：修改 inbounds 中的 clients，保留其他所有字段
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(x.Other, &raw); err != nil {
		return nil, err
	}
	inboundsBytes, err := json.Marshal(x.Inbounds)
	if err != nil {
		return nil, err
	}
	raw["inbounds"] = inboundsBytes
	return json.Marshal(raw)
}

func applyUsersToXrayConfig(cfg *xrayConfigTemplate, users map[string]*userEntry) ([]byte, error) {
	// 按 inbound_tag 分组用户
	usersByTag := make(map[string][]*userEntry)
	for _, u := range users {
		tag := "proxy" // 默认 inbound tag
		if u.Extra != nil {
			if t, ok := u.Extra["inbound_tag"].(string); ok && t != "" {
				tag = t
			}
		}
		usersByTag[tag] = append(usersByTag[tag], u)
	}

	for i := range cfg.Inbounds {
		inbound := &cfg.Inbounds[i]
		tag := inbound.Tag
		if tag == "" {
			tag = "proxy"
		}
		inboundUsers, ok := usersByTag[tag]
		if !ok {
			// 该 inbound 没有用户，保留现有 clients
			continue
		}

		// 解析 settings
		var settings map[string]json.RawMessage
		if len(inbound.Settings) > 0 {
			if err := json.Unmarshal(inbound.Settings, &settings); err != nil {
				continue
			}
		} else {
			settings = make(map[string]json.RawMessage)
		}

		// 构建新的 clients 数组
		clients := make([]json.RawMessage, 0, len(inboundUsers))
		for _, u := range inboundUsers {
			client := buildDeltaXrayClient(u)
			clientBytes, err := json.Marshal(client)
			if err != nil {
				continue
			}
			clients = append(clients, clientBytes)
		}
		clientsBytes, _ := json.Marshal(clients)
		settings["clients"] = clientsBytes

		settingsBytes, err := json.Marshal(settings)
		if err != nil {
			continue
		}
		inbound.Settings = settingsBytes
	}

	return json.Marshal(cfg)
}

func buildDeltaXrayClient(u *userEntry) map[string]interface{} {
	client := map[string]interface{}{
		"email": u.Email,
		"id":    u.UUID,
	}
	if u.Level > 0 {
		client["level"] = u.Level
	}
	if u.Password != "" {
		client["password"] = u.Password
	}
	if u.Extra != nil {
		for k, v := range u.Extra {
			if k == "inbound_tag" {
				continue
			}
			client[k] = v
		}
	}
	return client
}

// sing-box 配置结构。
type singboxConfigTemplate struct {
	Inbounds []singboxInboundTemplate `json:"inbounds"`
	Other    json.RawMessage          `json:"-"`
}

type singboxInboundTemplate struct {
	Type                string          `json:"type"`
	Tag                 string          `json:"tag,omitempty"`
	Listen              json.RawMessage `json:"listen,omitempty"`
	ListenPort          int             `json:"listen_port,omitempty"`
	Users               json.RawMessage `json:"users,omitempty"`
	Password            string          `json:"password,omitempty"`
	Other               map[string]json.RawMessage `json:"-"`
}

func (s *singboxConfigTemplate) UnmarshalJSON(data []byte) error {
	type alias singboxConfigTemplate
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*s = singboxConfigTemplate(a)
	s.Other = data
	return nil
}

func (s singboxConfigTemplate) MarshalJSON() ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(s.Other, &raw); err != nil {
		return nil, err
	}
	inboundsBytes, err := json.Marshal(s.Inbounds)
	if err != nil {
		return nil, err
	}
	raw["inbounds"] = inboundsBytes
	return json.Marshal(raw)
}

func applyUsersToSingboxConfig(cfg *singboxConfigTemplate, users map[string]*userEntry) ([]byte, error) {
	usersByTag := make(map[string][]*userEntry)
	for _, u := range users {
		tag := "proxy"
		if u.Extra != nil {
			if t, ok := u.Extra["inbound_tag"].(string); ok && t != "" {
				tag = t
			}
		}
		usersByTag[tag] = append(usersByTag[tag], u)
	}

	for i := range cfg.Inbounds {
		inbound := &cfg.Inbounds[i]
		tag := inbound.Tag
		if tag == "" {
			tag = "proxy"
		}
		inboundUsers, ok := usersByTag[tag]
		if !ok {
			continue
		}

		// 构建 sing-box users 数组
		sbUsers := make([]map[string]interface{}, 0, len(inboundUsers))
		for _, u := range inboundUsers {
			sbUser := buildSingboxUser(u, inbound.Type)
			sbUsers = append(sbUsers, sbUser)
		}
		usersBytes, _ := json.Marshal(sbUsers)
		inbound.Users = usersBytes
	}

	return json.Marshal(cfg)
}

func buildSingboxUser(u *userEntry, inboundType string) map[string]interface{} {
	user := map[string]interface{}{
		"name": u.Email,
	}
	switch inboundType {
	case "shadowsocks":
		user["password"] = u.Password
		if u.Extra != nil && u.Extra["method"] != nil {
			user["method"] = u.Extra["method"]
		}
	case "hysteria2", "tuic":
		user["password"] = u.Password
		if u.UUID != "" {
			user["uuid"] = u.UUID
		}
	default:
		// vless/vmess/trojan 使用 uuid
		if u.UUID != "" {
			user["uuid"] = u.UUID
		}
		if u.Password != "" {
			user["password"] = u.Password
		}
	}
	if u.Extra != nil {
		for k, v := range u.Extra {
			if k == "inbound_tag" || k == "method" {
				continue
			}
			user[k] = v
		}
	}
	return user
}

// extractUsersFromConfig 从配置 JSON 中提取用户列表。
func extractUsersFromConfig(configBytes []byte) (map[string]User, error) {
	users := make(map[string]User)

	// 尝试 Xray 格式
	var xrayCfg struct {
		Inbounds []struct {
			Tag      string `json:"tag"`
			Protocol string `json:"protocol"`
			Settings json.RawMessage `json:"settings"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(configBytes, &xrayCfg); err == nil {
		for _, in := range xrayCfg.Inbounds {
			// B16 修复：使用 map[string]interface{} 捕获所有协议字段（flow/encryption/alterId/method 等）
			var settings struct {
				Clients []map[string]interface{} `json:"clients"`
			}
			if err := json.Unmarshal(in.Settings, &settings); err != nil {
				continue
			}
			for _, c := range settings.Clients {
				email, _ := c["email"].(string)
				if email == "" {
					continue
				}
				uuid, _ := c["id"].(string)
				password, _ := c["password"].(string)
				level := 0
				if l, ok := c["level"].(float64); ok {
					level = int(l)
				}
				// 收集所有非标准字段到 Extra（保留协议专用字段）
				extra := map[string]interface{}{
					"inbound_tag": in.Tag,
					"protocol":    in.Protocol,
				}
				knownFields := map[string]bool{
					"email": true, "id": true, "password": true, "level": true,
				}
				for k, v := range c {
					if !knownFields[k] {
						extra[k] = v
					}
				}
				u := User{
					Email:    email,
					UUID:     uuid,
					Level:    level,
					Extra:    extra,
				}
				if password != "" {
					u.Password = password
				}
				users[email] = u
			}
		}
		if len(users) > 0 {
			return users, nil
		}
	}

	// 尝试 sing-box 格式
	var sbCfg struct {
		Inbounds []struct {
			Type  string `json:"type"`
			Tag   string `json:"tag"`
			Users []struct {
				Name     string                 `json:"name"`
				UUID     string                 `json:"uuid"`
				Password string                 `json:"password"`
				Extra    map[string]interface{} `json:"-"`
			} `json:"users"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(configBytes, &sbCfg); err == nil {
		for _, in := range sbCfg.Inbounds {
			for _, u := range in.Users {
				if u.Name == "" {
					continue
				}
				user := User{
					Email: u.Name,
					UUID:  u.UUID,
					Password: u.Password,
					Extra: map[string]interface{}{
						"inbound_tag": in.Tag,
						"protocol":    in.Type,
					},
				}
				users[u.Name] = user
			}
		}
	}

	return users, nil
}
