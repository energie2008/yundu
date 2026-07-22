-- +goose Up
-- +goose StatementBegin
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS server_port INTEGER DEFAULT NULL;
COMMENT ON COLUMN nodes.server_port IS 'xray/sing-box 本地监听的服务端口（高位端口，nginx SNI 转发目标）。CDN/Tunnel/REALITY 节点必填，UDP 直连节点为 NULL';
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS reality_server_name TEXT DEFAULT NULL;
COMMENT ON COLUMN nodes.reality_server_name IS 'REALITY 节点的伪装 SNI 域名，用于 nginx stream SNI 分流';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE nodes DROP COLUMN IF EXISTS server_port;
ALTER TABLE nodes DROP COLUMN IF EXISTS reality_server_name;
-- +goose StatementEnd
