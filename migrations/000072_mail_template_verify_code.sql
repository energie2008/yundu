-- 补充 verify_code 邮件模板（注册邮箱验证码）。
-- 000052 只内置了 verify_email 等 6 个模板，缺少 verify_code，
-- 导致注册“获取验证码”报 mail template not found。
INSERT INTO mail_templates (name, subject, body, is_builtin, enabled)
VALUES (
    'verify_code',
    '{{.SiteName}} - 邮箱验证码',
    '<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>邮箱验证码</title></head>
<body style="margin:0;padding:0;background-color:#f4f4f5;font-family:-apple-system,BlinkMacSystemFont,''Segoe UI'',Roboto,''Helvetica Neue'',Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f4f4f5;padding:40px 20px;">
<tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
    <tr><td style="padding-bottom:24px;text-align:center;">
        <span style="font-size:20px;font-weight:700;color:#18181b;">{{.SiteName}}</span>
    </td></tr>
    <tr><td style="background:#ffffff;border-radius:12px;border:1px solid #e4e4e7;padding:40px;">
        <table width="100%" cellpadding="0" cellspacing="0">
            <tr><td style="font-size:22px;font-weight:700;color:#18181b;padding-bottom:8px;">邮箱验证码</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.6;padding-bottom:12px;">您好，{{.UserName}}</td></tr>
            <tr><td style="font-size:15px;color:#52525b;line-height:1.6;padding-bottom:24px;">您正在注册 {{.SiteName}}，本次验证码为：</td></tr>
            <tr><td align="center" style="padding-bottom:24px;">
                <span style="display:inline-block;background:#f4f4f5;border:1px dashed #c7c7cc;border-radius:10px;padding:14px 28px;font-size:28px;font-weight:700;letter-spacing:6px;color:#18181b;">{{.Code}}</span>
            </td></tr>
            <tr><td style="font-size:13px;color:#a1a1aa;line-height:1.6;">验证码 10 分钟内有效，请勿泄露给他人。如果您没有注册，请忽略此邮件。</td></tr>
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
</html>',
    TRUE,
    TRUE
)
ON CONFLICT (name) DO UPDATE SET
    subject = EXCLUDED.subject,
    body = EXCLUDED.body,
    is_builtin = TRUE,
    enabled = TRUE,
    updated_at = now();
