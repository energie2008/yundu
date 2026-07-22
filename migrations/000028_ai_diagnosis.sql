-- +goose Up
-- +goose StatementBegin

-- ============================================================================
-- AI 诊断知识库：经验沉淀与自动修复规则
-- ============================================================================
CREATE TABLE IF NOT EXISTS diagnosis_knowledge (
  id UUID PRIMARY KEY,
  title VARCHAR(256) NOT NULL,
  category VARCHAR(64) NOT NULL,
  root_cause_pattern TEXT NOT NULL,
  solution TEXT NOT NULL,
  auto_fix_action TEXT,
  doc_links JSONB NOT NULL DEFAULT '[]',
  hit_count INTEGER NOT NULL DEFAULT 0,
  is_verified BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_diagnosis_knowledge_category ON diagnosis_knowledge(category, is_verified, hit_count DESC);

-- ============================================================================
-- AI 智能诊断：诊断会话表
-- ============================================================================
CREATE TABLE IF NOT EXISTS diagnosis_sessions (
  id UUID PRIMARY KEY,
  server_id UUID REFERENCES servers(id) ON DELETE SET NULL,
  node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  trigger_source VARCHAR(32) NOT NULL DEFAULT 'admin',
  time_window_start TIMESTAMPTZ,
  time_window_end TIMESTAMPTZ,
  raw_logs TEXT,
  raw_metrics JSONB NOT NULL DEFAULT '{}',
  llm_provider VARCHAR(32) NOT NULL DEFAULT '',
  llm_model VARCHAR(64) NOT NULL DEFAULT '',
  root_cause_category VARCHAR(64),
  root_cause_description TEXT,
  confidence DOUBLE PRECISION,
  suggestions JSONB NOT NULL DEFAULT '[]',
  doc_links JSONB NOT NULL DEFAULT '[]',
  knowledge_entry_id UUID REFERENCES diagnosis_knowledge(id) ON DELETE SET NULL,
  autofix_applied BOOLEAN NOT NULL DEFAULT false,
  autofix_result JSONB,
  duration_ms INTEGER,
  created_by_admin_id UUID REFERENCES admins(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_diagnosis_sessions_node ON diagnosis_sessions(node_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_diagnosis_sessions_server ON diagnosis_sessions(server_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_diagnosis_sessions_status ON diagnosis_sessions(status, created_at DESC);

INSERT INTO diagnosis_knowledge (id, title, category, root_cause_pattern, solution, auto_fix_action, is_verified) VALUES
  ('50000000-0000-0000-0000-000000000001', '节点gRPC连接频繁断开', 'connectivity', 'grpc.*connection.*refused|connection reset|broken pipe', '检查节点防火墙规则，确认9082端口可达；检查node-agent进程是否正常运行；查看服务端与客户端TLS证书是否匹配', NULL, true),
  ('50000000-0000-0000-0000-000000000002', 'TLS证书过期', 'certificate', 'certificate has expired|tls: expired|x509: certificate has expired', '通过admin面板触发证书续签，或手动更新cert目录下的PEM文件后重启节点服务', 'renew_cert', true),
  ('50000000-0000-0000-0000-000000000003', '内存使用率过高', 'resource', 'out of memory|OOM|memory.*high|heap alloc', '检查是否有内存泄漏，考虑增加节点内存或限制单连接最大并发数；检查sing-box/xray配置中的连接数限制', NULL, true)
ON CONFLICT (id) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS diagnosis_sessions CASCADE;
DROP TABLE IF EXISTS diagnosis_knowledge CASCADE;
-- +goose StatementEnd
