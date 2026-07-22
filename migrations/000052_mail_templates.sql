-- +goose Up
-- +goose StatementBegin
-- 邮件模板系统（参考 Xboard v2_mail_templates）
-- 支持 6 种内置模板：verify_email, reset_password, payment_success,
-- ticket_reply, subscription_expired, traffic_warning
-- 模板使用 Go html/template 语法：{{.UserName}}, {{.VerifyURL}} 等

CREATE TABLE IF NOT EXISTS mail_templates (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(64) NOT NULL UNIQUE,
    subject     VARCHAR(500) NOT NULL DEFAULT '',
    body        TEXT NOT NULL DEFAULT '',
    is_builtin  BOOLEAN NOT NULL DEFAULT FALSE,
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_mail_templates_name ON mail_templates(name) WHERE enabled = TRUE;

-- ============================================
-- 1. verify_email - 注册邮箱验证
-- 变量: UserName, VerifyURL, SiteName, SiteURL
-- ============================================
INSERT INTO mail_templates (name, subject, body, is_builtin, enabled) VALUES
('verify_email',
 '{{.SiteName}} - 邮箱验证',
 $$<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>邮箱验证</title></head>
<body style="margin:0;padding:0;background-color:#f4f4f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f4f4f5;padding:40px 20px;">
<tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
    <tr><td style="padding-bottom:24px;text-align:center;">
        <span style="font-size:20px;font-weight:700;color:#18181b;">{{.SiteName}}</span>
    </td></tr>
    <tr><td style="background:#ffffff;border-radius:12px;border:1px solid #e4e4e7;padding:40px;">
        <table width="100%" cellpadding="0" cellspacing="0">
            <tr><td style="font-size:22px;font-weight:700;color:#18181b;padding-bottom:8px;">邮箱验证</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.6;padding-bottom:12px;">您好，{{.UserName}}</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.6;padding-bottom:28px;">感谢您注册 {{.SiteName}}。请点击下方按钮验证您的邮箱地址：</td></tr>
            <tr><td align="center" style="padding-bottom:28px;">
                <a href="{{.VerifyURL}}" style="display:inline-block;background:#4F46E5;color:#ffffff;font-size:14px;font-weight:600;text-decoration:none;padding:12px 30px;border-radius:8px;">验证邮箱</a>
            </td></tr>
            <tr><td style="font-size:13px;color:#a1a1aa;line-height:1.5;">或复制以下链接到浏览器打开：</td></tr>
            <tr><td style="font-size:13px;color:#a1a1aa;line-height:1.5;word-break:break-all;padding-bottom:16px;">{{.VerifyURL}}</td></tr>
            <tr><td style="font-size:13px;color:#a1a1aa;line-height:1.5;">该链接有效期为 24 小时。如果您没有注册，请忽略此邮件。</td></tr>
        </table>
    </td></tr>
    <tr><td style="padding-top:24px;text-align:center;">
        <a href="{{.SiteURL}}" style="font-size:13px;color:#a1a1aa;text-decoration:none;">{{.SiteURL}}</a>
        <p style="font-size:12px;color:#d4d4d8;margin:8px 0 0;">此邮件由系统自动发送，请勿直接回复。</p>
    </td></tr>
</table>
</td></tr>
</table>
</body>
</html>$$,
 TRUE, TRUE)
ON CONFLICT (name) DO NOTHING;

-- ============================================
-- 2. reset_password - 密码重置
-- 变量: UserName, ResetURL, SiteName, SiteURL
-- ============================================
INSERT INTO mail_templates (name, subject, body, is_builtin, enabled) VALUES
('reset_password',
 '{{.SiteName}} - 重置密码',
 $$<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>重置密码</title></head>
<body style="margin:0;padding:0;background-color:#f4f4f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f4f4f5;padding:40px 20px;">
<tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
    <tr><td style="padding-bottom:24px;text-align:center;">
        <span style="font-size:20px;font-weight:700;color:#18181b;">{{.SiteName}}</span>
    </td></tr>
    <tr><td style="background:#ffffff;border-radius:12px;border:1px solid #e4e4e7;padding:40px;">
        <table width="100%" cellpadding="0" cellspacing="0">
            <tr><td style="font-size:22px;font-weight:700;color:#18181b;padding-bottom:8px;">重置密码</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.6;padding-bottom:12px;">您好，{{.UserName}}</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.6;padding-bottom:28px;">我们收到了您的密码重置请求。请点击下方按钮重置您的密码：</td></tr>
            <tr><td align="center" style="padding-bottom:28px;">
                <a href="{{.ResetURL}}" style="display:inline-block;background:#4F46E5;color:#ffffff;font-size:14px;font-weight:600;text-decoration:none;padding:12px 30px;border-radius:8px;">重置密码</a>
            </td></tr>
            <tr><td style="font-size:13px;color:#a1a1aa;line-height:1.5;">或复制以下链接到浏览器打开：</td></tr>
            <tr><td style="font-size:13px;color:#a1a1aa;line-height:1.5;word-break:break-all;padding-bottom:16px;">{{.ResetURL}}</td></tr>
            <tr><td style="font-size:13px;color:#a1a1aa;line-height:1.5;">该链接有效期为 1 小时。如果您没有请求重置密码，请忽略此邮件。</td></tr>
        </table>
    </td></tr>
    <tr><td style="padding-top:24px;text-align:center;">
        <a href="{{.SiteURL}}" style="font-size:13px;color:#a1a1aa;text-decoration:none;">{{.SiteURL}}</a>
        <p style="font-size:12px;color:#d4d4d8;margin:8px 0 0;">此邮件由系统自动发送，请勿直接回复。</p>
    </td></tr>
</table>
</td></tr>
</table>
</body>
</html>$$,
 TRUE, TRUE)
ON CONFLICT (name) DO NOTHING;

-- ============================================
-- 3. payment_success - 支付成功通知
-- 变量: UserName, OrderID, Amount, SiteName, SiteURL
-- ============================================
INSERT INTO mail_templates (name, subject, body, is_builtin, enabled) VALUES
('payment_success',
 '{{.SiteName}} - 支付成功',
 $$<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>支付成功</title></head>
<body style="margin:0;padding:0;background-color:#f4f4f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f4f4f5;padding:40px 20px;">
<tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
    <tr><td style="padding-bottom:24px;text-align:center;">
        <span style="font-size:20px;font-weight:700;color:#18181b;">{{.SiteName}}</span>
    </td></tr>
    <tr><td style="background:#ffffff;border-radius:12px;border:1px solid #e4e4e7;padding:40px;">
        <table width="100%" cellpadding="0" cellspacing="0">
            <tr><td style="font-size:22px;font-weight:700;color:#18181b;padding-bottom:8px;">支付成功</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.6;padding-bottom:12px;">您好，{{.UserName}}</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.7;padding-bottom:12px;">您的订单 <strong style="color:#18181b;">{{.OrderID}}</strong> 已支付成功，支付金额 <strong style="color:#18181b;">{{.Amount}} USDT</strong>。</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.7;padding-bottom:28px;">您的订阅已自动开通/续期，请登录面板查看详情。</td></tr>
            <tr><td align="center">
                <a href="{{.SiteURL}}" style="display:inline-block;background:#18181b;color:#ffffff;font-size:14px;font-weight:600;text-decoration:none;padding:12px 28px;border-radius:8px;">查看订单</a>
            </td></tr>
        </table>
    </td></tr>
    <tr><td style="padding-top:24px;text-align:center;">
        <a href="{{.SiteURL}}" style="font-size:13px;color:#a1a1aa;text-decoration:none;">{{.SiteURL}}</a>
        <p style="font-size:12px;color:#d4d4d8;margin:8px 0 0;">此邮件由系统自动发送，请勿直接回复。</p>
    </td></tr>
</table>
</td></tr>
</table>
</body>
</html>$$,
 TRUE, TRUE)
ON CONFLICT (name) DO NOTHING;

-- ============================================
-- 4. ticket_reply - 工单回复通知
-- 变量: UserName, TicketSubject, ReplyContent, SiteName, SiteURL
-- ============================================
INSERT INTO mail_templates (name, subject, body, is_builtin, enabled) VALUES
('ticket_reply',
 '{{.SiteName}} - 工单回复通知',
 $$<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>工单回复通知</title></head>
<body style="margin:0;padding:0;background-color:#f4f4f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f4f4f5;padding:40px 20px;">
<tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
    <tr><td style="padding-bottom:24px;text-align:center;">
        <span style="font-size:20px;font-weight:700;color:#18181b;">{{.SiteName}}</span>
    </td></tr>
    <tr><td style="background:#ffffff;border-radius:12px;border:1px solid #e4e4e7;padding:40px;">
        <table width="100%" cellpadding="0" cellspacing="0">
            <tr><td style="font-size:22px;font-weight:700;color:#18181b;padding-bottom:8px;">工单回复通知</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.6;padding-bottom:12px;">您好，{{.UserName}}</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.7;padding-bottom:12px;">您的工单「<strong style="color:#18181b;">{{.TicketSubject}}</strong>」收到了新的回复：</td></tr>
            <tr><td style="background:#f4f4f5;border-radius:8px;padding:16px 20px;font-size:14px;color:#52525b;line-height:1.6;margin-bottom:28px;">{{.ReplyContent}}</td></tr>
            <tr><td align="center" style="padding-bottom:8px;">
                <a href="{{.SiteURL}}" style="display:inline-block;background:#18181b;color:#ffffff;font-size:14px;font-weight:600;text-decoration:none;padding:12px 28px;border-radius:8px;">查看工单</a>
            </td></tr>
        </table>
    </td></tr>
    <tr><td style="padding-top:24px;text-align:center;">
        <a href="{{.SiteURL}}" style="font-size:13px;color:#a1a1aa;text-decoration:none;">{{.SiteURL}}</a>
        <p style="font-size:12px;color:#d4d4d8;margin:8px 0 0;">此邮件由系统自动发送，请勿直接回复。</p>
    </td></tr>
</table>
</td></tr>
</table>
</body>
</html>$$,
 TRUE, TRUE)
ON CONFLICT (name) DO NOTHING;

-- ============================================
-- 5. subscription_expired - 订阅到期提醒
-- 变量: UserName, PlanName, ExpireDate, SiteName, SiteURL
-- ============================================
INSERT INTO mail_templates (name, subject, body, is_builtin, enabled) VALUES
('subscription_expired',
 '{{.SiteName}} - 订阅即将到期',
 $$<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>订阅到期提醒</title></head>
<body style="margin:0;padding:0;background-color:#f4f4f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f4f4f5;padding:40px 20px;">
<tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
    <tr><td style="padding-bottom:24px;text-align:center;">
        <span style="font-size:20px;font-weight:700;color:#18181b;">{{.SiteName}}</span>
    </td></tr>
    <tr><td style="background:#ffffff;border-radius:12px;border:1px solid #e4e4e7;padding:40px;">
        <table width="100%" cellpadding="0" cellspacing="0">
            <tr><td style="font-size:22px;font-weight:700;color:#18181b;padding-bottom:8px;">订阅即将到期</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.6;padding-bottom:12px;">您好，{{.UserName}}</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.7;padding-bottom:12px;">您的订阅「<strong style="color:#18181b;">{{.PlanName}}</strong>」将于 <strong style="color:#18181b;">{{.ExpireDate}}</strong> 到期。</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.7;padding-bottom:28px;">为避免服务中断，请及时续费。如您已完成续费，请忽略此提醒。</td></tr>
            <tr><td align="center">
                <a href="{{.SiteURL}}" style="display:inline-block;background:#18181b;color:#ffffff;font-size:14px;font-weight:600;text-decoration:none;padding:12px 28px;border-radius:8px;">立即续费</a>
            </td></tr>
        </table>
    </td></tr>
    <tr><td style="padding-top:24px;text-align:center;">
        <a href="{{.SiteURL}}" style="font-size:13px;color:#a1a1aa;text-decoration:none;">{{.SiteURL}}</a>
        <p style="font-size:12px;color:#d4d4d8;margin:8px 0 0;">此邮件由系统自动发送，请勿直接回复。</p>
    </td></tr>
</table>
</td></tr>
</table>
</body>
</html>$$,
 TRUE, TRUE)
ON CONFLICT (name) DO NOTHING;

-- ============================================
-- 6. traffic_warning - 流量告警（80% 阈值）
-- 变量: UserName, TrafficUsed, TrafficTotal, SiteName, SiteURL
-- ============================================
INSERT INTO mail_templates (name, subject, body, is_builtin, enabled) VALUES
('traffic_warning',
 '{{.SiteName}} - 流量使用提醒',
 $$<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>流量使用提醒</title></head>
<body style="margin:0;padding:0;background-color:#f4f4f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f4f4f5;padding:40px 20px;">
<tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
    <tr><td style="padding-bottom:24px;text-align:center;">
        <span style="font-size:20px;font-weight:700;color:#18181b;">{{.SiteName}}</span>
    </td></tr>
    <tr><td style="background:#ffffff;border-radius:12px;border:1px solid #e4e4e7;padding:40px;">
        <table width="100%" cellpadding="0" cellspacing="0">
            <tr><td style="font-size:22px;font-weight:700;color:#18181b;padding-bottom:8px;">流量使用提醒</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.6;padding-bottom:12px;">您好，{{.UserName}}</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.7;padding-bottom:12px;">您本月的套餐流量已使用 <strong style="color:#18181b;">{{.TrafficUsed}}</strong> / <strong style="color:#18181b;">{{.TrafficTotal}}</strong>，使用率已达到 80%。</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.7;padding-bottom:28px;">请合理安排使用，避免提前耗尽。如需更多流量，可前往面板升级套餐。</td></tr>
            <tr><td align="center">
                <a href="{{.SiteURL}}" style="display:inline-block;background:#18181b;color:#ffffff;font-size:14px;font-weight:600;text-decoration:none;padding:12px 28px;border-radius:8px;">查看用量</a>
            </td></tr>
        </table>
    </td></tr>
    <tr><td style="padding-top:24px;text-align:center;">
        <a href="{{.SiteURL}}" style="font-size:13px;color:#a1a1aa;text-decoration:none;">{{.SiteURL}}</a>
        <p style="font-size:12px;color:#d4d4d8;margin:8px 0 0;">此邮件由系统自动发送，请勿直接回复。</p>
    </td></tr>
</table>
</td></tr>
</table>
</body>
</html>$$,
 TRUE, TRUE)
ON CONFLICT (name) DO NOTHING;

-- 种子数据：邮件模板管理权限
INSERT INTO permissions (code, name, resource, action) VALUES
('mail.read',  '查看邮件模板', 'mail', 'read'),
('mail.write', '管理邮件模板', 'mail', 'write')
ON CONFLICT (code) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM permissions WHERE code IN ('mail.read', 'mail.write');
DROP TABLE IF EXISTS mail_templates;
-- +goose StatementEnd
