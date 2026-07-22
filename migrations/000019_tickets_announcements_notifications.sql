-- +goose Up
-- +goose StatementBegin
-- Phase 6: 工单 / 公告 / 通知模块
-- 1) tickets + ticket_replies
-- 2) announcements + announcement_reads
-- 3) notifications + notification_templates

-- ============================================
-- 1. 工单 tickets
-- ============================================
CREATE TABLE IF NOT EXISTS tickets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subject         VARCHAR(200) NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    category        VARCHAR(32) NOT NULL DEFAULT 'consultation',
    priority        VARCHAR(32) NOT NULL DEFAULT 'normal',
    status          VARCHAR(32) NOT NULL DEFAULT 'open',
    assigned_admin_id UUID REFERENCES users(id) ON DELETE SET NULL,
    related_resource_type VARCHAR(64),
    related_resource_id   UUID,
    reply_count     INTEGER NOT NULL DEFAULT 0,
    last_reply_at   TIMESTAMPTZ,
    closed_at       TIMESTAMPTZ,
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX idx_tickets_user_id ON tickets(user_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_tickets_status ON tickets(status, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_tickets_assigned ON tickets(assigned_admin_id) WHERE deleted_at IS NULL AND assigned_admin_id IS NOT NULL;
CREATE INDEX idx_tickets_category ON tickets(category) WHERE deleted_at IS NULL;

-- ============================================
-- 工单回复 ticket_replies
-- ============================================
CREATE TABLE IF NOT EXISTS ticket_replies (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id       UUID NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    author_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    author_type     VARCHAR(16) NOT NULL DEFAULT 'user',
    content         TEXT NOT NULL DEFAULT '',
    attachments     JSONB NOT NULL DEFAULT '[]'::jsonb,
    is_internal     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ticket_replies_ticket_id ON ticket_replies(ticket_id, created_at ASC);

-- ============================================
-- 2. 公告 announcements
-- ============================================
CREATE TABLE IF NOT EXISTS announcements (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title           VARCHAR(200) NOT NULL,
    content         TEXT NOT NULL DEFAULT '',
    summary         VARCHAR(500),
    type            VARCHAR(32) NOT NULL DEFAULT 'notice',
    status          VARCHAR(32) NOT NULL DEFAULT 'draft',
    target_audience VARCHAR(32) NOT NULL DEFAULT 'all',
    target_filter   JSONB NOT NULL DEFAULT '{}'::jsonb,
    effective_at    TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ,
    pinned          BOOLEAN NOT NULL DEFAULT FALSE,
    view_count      INTEGER NOT NULL DEFAULT 0,
    read_count      INTEGER NOT NULL DEFAULT 0,
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    published_at    TIMESTAMPTZ,
    archived_at     TIMESTAMPTZ,
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX idx_announcements_status_type ON announcements(status, type, effective_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_announcements_pinned ON announcements(pinned, created_at DESC) WHERE deleted_at IS NULL AND status = 'published';
CREATE INDEX idx_announcements_target ON announcements(target_audience) WHERE deleted_at IS NULL AND status = 'published';

-- 公告已读
CREATE TABLE IF NOT EXISTS announcement_reads (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    announcement_id UUID NOT NULL REFERENCES announcements(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    read_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(announcement_id, user_id)
);

CREATE INDEX idx_announcement_reads_user ON announcement_reads(user_id, read_at DESC);

-- ============================================
-- 3. 通知 notifications
-- ============================================
CREATE TABLE IF NOT EXISTS notifications (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category        VARCHAR(64) NOT NULL DEFAULT 'system',
    title           VARCHAR(200) NOT NULL,
    content         TEXT NOT NULL DEFAULT '',
    channel         VARCHAR(32) NOT NULL DEFAULT 'in_app',
    status          VARCHAR(32) NOT NULL DEFAULT 'pending',
    priority        VARCHAR(32) NOT NULL DEFAULT 'normal',
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    target_resource_type VARCHAR(64),
    target_resource_id   UUID,
    template_code   VARCHAR(64),
    scheduled_at    TIMESTAMPTZ,
    sent_at         TIMESTAMPTZ,
    read_at         TIMESTAMPTZ,
    archived_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notifications_user_unread ON notifications(user_id, created_at DESC) WHERE read_at IS NULL AND archived_at IS NULL;
CREATE INDEX idx_notifications_user_all ON notifications(user_id, created_at DESC);
CREATE INDEX idx_notifications_status_scheduled ON notifications(status, scheduled_at) WHERE status = 'pending' AND scheduled_at IS NOT NULL;
CREATE INDEX idx_notifications_category ON notifications(category, created_at DESC);

-- ============================================
-- 通知模板 notification_templates
-- ============================================
CREATE TABLE IF NOT EXISTS notification_templates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code            VARCHAR(64) NOT NULL UNIQUE,
    name            VARCHAR(200) NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    category        VARCHAR(64) NOT NULL DEFAULT 'system',
    channel         VARCHAR(32) NOT NULL DEFAULT 'in_app',
    title_template  TEXT NOT NULL DEFAULT '',
    body_template   TEXT NOT NULL DEFAULT '',
    variables       JSONB NOT NULL DEFAULT '[]'::jsonb,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notification_templates_category ON notification_templates(category, enabled);

-- 种子数据：通知模板
INSERT INTO notification_templates (code, name, description, category, channel, title_template, body_template, variables) VALUES
('traffic_expiry', '流量到期提醒', '当用户流量即将用尽时触发', 'traffic_expiry', 'in_app',
 '流量即将用尽', '您的套餐剩余流量已不足 {{remaining_bytes}} MB，请及时续费或升级套餐。',
 '[{"key":"remaining_bytes","label":"剩余流量(MB)","type":"number"}]'),
('plan_expiry', '套餐到期提醒', '当用户套餐即将过期时触发', 'plan_expiry', 'in_app',
 '套餐即将到期', '您的套餐将于 {{expiry_date}} 到期，请及时续费。',
 '[{"key":"expiry_date","label":"到期日期","type":"string"}]'),
('ticket_replied', '工单回复提醒', '用户工单有新回复时触发', 'ticket', 'in_app',
 '工单有新回复', '您的工单「{{ticket_subject}}」有新回复。',
 '[{"key":"ticket_subject","label":"工单主题","type":"string"}]'),
('announcement_published', '新公告提醒', '新公告发布时触发', 'announcement', 'in_app',
 '新公告：{{announcement_title}}', '{{announcement_summary}}',
 '[{"key":"announcement_title","label":"公告标题","type":"string"},{"key":"announcement_summary","label":"公告摘要","type":"string"}]')
ON CONFLICT (code) DO NOTHING;

-- 种子数据：权限（RBAC 表存在）
INSERT INTO permissions (code, name, resource, action) VALUES
('tickets.read',           '查看工单',      'tickets',        'read'),
('tickets.write',          '处理工单',      'tickets',        'write'),
('announcements.read',     '查看公告',      'announcements',  'read'),
('announcements.write',    '管理公告',      'announcements',  'write'),
('notifications.read',     '查看通知',      'notifications',  'read'),
('notifications.write',   '管理通知模板',  'notifications',  'write')
ON CONFLICT (code) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS notification_templates;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS announcement_reads;
DROP TABLE IF EXISTS announcements;
DROP TABLE IF EXISTS ticket_replies;
DROP TABLE IF EXISTS tickets;

DELETE FROM permissions WHERE code IN (
    'tickets.read', 'tickets.write',
    'announcements.read', 'announcements.write',
    'notifications.read', 'notifications.write'
);
-- +goose StatementEnd
