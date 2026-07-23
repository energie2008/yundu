-- +goose Up
-- +goose StatementBegin
-- 邮箱验证码注册功能（参考 Xboard CommController::sendEmailVerify）
-- 1. 注册验证码邮件模板（变量: UserName, Code, SiteName, SiteURL）
INSERT INTO mail_templates (name, subject, body, is_builtin, enabled) VALUES
('verify_code',
 '{{.SiteName}} - 注册验证码',
 $$<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>注册验证码</title></head>
<body style="margin:0;padding:0;background-color:#f4f4f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f4f4f5;padding:40px 20px;">
<tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
    <tr><td style="padding-bottom:24px;text-align:center;">
        <span style="font-size:20px;font-weight:700;color:#18181b;">{{.SiteName}}</span>
    </td></tr>
    <tr><td style="background:#ffffff;border-radius:12px;border:1px solid #e4e4e7;padding:40px;">
        <table width="100%" cellpadding="0" cellspacing="0">
            <tr><td style="font-size:22px;font-weight:700;color:#18181b;padding-bottom:8px;">注册验证码</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.6;padding-bottom:12px;">您好，{{.UserName}}</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.6;padding-bottom:28px;">感谢您注册 {{.SiteName}}。您的注册验证码为：</td></tr>
            <tr><td align="center" style="padding-bottom:28px;">
                <span style="display:inline-block;background:#f4f4f5;border:1px solid #e4e4e7;border-radius:8px;font-size:32px;font-weight:700;letter-spacing:8px;color:#4F46E5;padding:16px 36px;">{{.Code}}</span>
            </td></tr>
            <tr><td style="font-size:13px;color:#a1a1aa;line-height:1.5;">该验证码有效期为 5 分钟，请尽快使用。如果您没有注册账号，请忽略此邮件。</td></tr>
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

-- 2. 系统设置：注册强制邮箱验证码（默认开启）
--    group=register, key=email_verify_required, value_json=true
INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
SELECT 'register', 'email_verify_required', 'true'::jsonb, false, '注册时强制邮箱验证码验证（关闭则回退到链接验证流程）'
WHERE NOT EXISTS (
    SELECT 1 FROM system_settings WHERE setting_group = 'register' AND setting_key = 'email_verify_required'
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM mail_templates WHERE name = 'verify_code';
DELETE FROM system_settings WHERE setting_group = 'register' AND setting_key = 'email_verify_required';
-- +goose StatementEnd
