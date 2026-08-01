-- 邮件模板整体美化 + 站点名称/地址接入系统设置。
-- 品牌名固定展示为 yundu云渡服务；站点地址读取 general.frontend_url（系统设置可随时修改）。

-- 站点设置（邮件模板 {{.SiteName}} / {{.SiteURL}} 的真实数据源）
INSERT INTO system_settings (setting_group, setting_key, value_json, description, is_secret)
VALUES
    ('general', 'app_name', '"yundu云渡服务"', '站点名称（邮件模板 {{.SiteName}}）', FALSE),
    ('general', 'frontend_url', '"https://7.tiktokplay.na.am"', '站点地址（邮件模板 {{.SiteURL}}）', FALSE)
ON CONFLICT (setting_group, setting_key) DO UPDATE SET
    value_json = EXCLUDED.value_json,
    description = EXCLUDED.description,
    is_secret = FALSE,
    updated_at = now();

-- 1. 注册邮箱验证
UPDATE mail_templates SET
    subject = '{{.SiteName}} - 验证您的邮箱',
    body = $$<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>邮箱验证</title></head>
<body style="margin:0;padding:0;background-color:#f3f4f6;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f3f4f6;padding:40px 16px;"><tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
  <tr><td align="center" style="padding-bottom:24px;">
    <span style="font-size:22px;font-weight:800;color:#18181b;">yundu云渡服务</span>
    <p style="margin:4px 0 0;font-size:13px;color:#8b8d98;">稳定 · 高速 · 全球化网络加速</p>
  </td></tr>
  <tr><td style="background:#ffffff;border-radius:14px;border:1px solid #e5e7eb;padding:36px 32px;">
    <table width="100%" cellpadding="0" cellspacing="0">
      <tr><td style="font-size:22px;font-weight:800;color:#18181b;padding-bottom:8px;">验证您的邮箱</td></tr>
      <tr><td style="font-size:15px;color:#4b5563;line-height:1.7;padding-bottom:10px;">您好，{{.UserName}}</td></tr>
      <tr><td style="font-size:15px;color:#4b5563;line-height:1.7;padding-bottom:28px;">感谢您注册 {{.SiteName}}。请点击下方按钮完成邮箱验证，验证后即可开始使用服务：</td></tr>
      <tr><td align="center" style="padding-bottom:28px;">
        <a href="{{.VerifyURL}}" style="display:inline-block;background:#4f46e5;color:#ffffff;font-size:14px;font-weight:700;text-decoration:none;padding:13px 34px;border-radius:10px;">验证邮箱</a>
      </td></tr>
      <tr><td style="font-size:13px;color:#9ca3af;line-height:1.6;">如果按钮无法点击，请复制以下链接到浏览器打开：</td></tr>
      <tr><td style="font-size:13px;color:#6b7280;line-height:1.6;word-break:break-all;padding-bottom:16px;">{{.VerifyURL}}</td></tr>
      <tr><td style="font-size:12px;color:#9ca3af;line-height:1.6;">该链接 24 小时内有效。如果您没有注册 {{.SiteName}}，请忽略此邮件。</td></tr>
    </table>
  </td></tr>
  <tr><td align="center" style="padding-top:24px;">
    <a href="{{.SiteURL}}" style="font-size:13px;color:#8b8d98;text-decoration:none;">{{.SiteURL}}</a>
    <p style="font-size:12px;color:#b0b3ba;margin:8px 0 0;">此邮件由 {{.SiteName}} 系统自动发送，请勿直接回复。</p>
  </td></tr>
</table>
</td></tr></table>
</body>
</html>$$,
    updated_at = now()
WHERE name = 'verify_email';

-- 2. 重置密码
UPDATE mail_templates SET
    subject = '{{.SiteName}} - 重置您的密码',
    body = $$<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>重置密码</title></head>
<body style="margin:0;padding:0;background-color:#f3f4f6;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f3f4f6;padding:40px 16px;"><tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
  <tr><td align="center" style="padding-bottom:24px;">
    <span style="font-size:22px;font-weight:800;color:#18181b;">yundu云渡服务</span>
    <p style="margin:4px 0 0;font-size:13px;color:#8b8d98;">稳定 · 高速 · 全球化网络加速</p>
  </td></tr>
  <tr><td style="background:#ffffff;border-radius:14px;border:1px solid #e5e7eb;padding:36px 32px;">
    <table width="100%" cellpadding="0" cellspacing="0">
      <tr><td style="font-size:22px;font-weight:800;color:#18181b;padding-bottom:8px;">重置您的密码</td></tr>
      <tr><td style="font-size:15px;color:#4b5563;line-height:1.7;padding-bottom:10px;">您好，{{.UserName}}</td></tr>
      <tr><td style="font-size:15px;color:#4b5563;line-height:1.7;padding-bottom:28px;">我们收到了重置 {{.SiteName}} 账号密码的请求。请点击下方按钮设置新密码：</td></tr>
      <tr><td align="center" style="padding-bottom:28px;">
        <a href="{{.ResetURL}}" style="display:inline-block;background:#18181b;color:#ffffff;font-size:14px;font-weight:700;text-decoration:none;padding:13px 34px;border-radius:10px;">重置密码</a>
      </td></tr>
      <tr><td style="font-size:13px;color:#9ca3af;line-height:1.6;">如果按钮无法点击，请复制以下链接到浏览器打开：</td></tr>
      <tr><td style="font-size:13px;color:#6b7280;line-height:1.6;word-break:break-all;padding-bottom:16px;">{{.ResetURL}}</td></tr>
      <tr><td style="font-size:12px;color:#9ca3af;line-height:1.6;">该链接 30 分钟内有效。如果不是您本人操作，请忽略此邮件并注意账号安全。</td></tr>
    </table>
  </td></tr>
  <tr><td align="center" style="padding-top:24px;">
    <a href="{{.SiteURL}}" style="font-size:13px;color:#8b8d98;text-decoration:none;">{{.SiteURL}}</a>
    <p style="font-size:12px;color:#b0b3ba;margin:8px 0 0;">此邮件由 {{.SiteName}} 系统自动发送，请勿直接回复。</p>
  </td></tr>
</table>
</td></tr></table>
</body>
</html>$$,
    updated_at = now()
WHERE name = 'reset_password';

-- 3. 支付成功
UPDATE mail_templates SET
    subject = '{{.SiteName}} - 支付成功通知',
    body = $$<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>支付成功</title></head>
<body style="margin:0;padding:0;background-color:#f3f4f6;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f3f4f6;padding:40px 16px;"><tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
  <tr><td align="center" style="padding-bottom:24px;">
    <span style="font-size:22px;font-weight:800;color:#18181b;">yundu云渡服务</span>
    <p style="margin:4px 0 0;font-size:13px;color:#8b8d98;">稳定 · 高速 · 全球化网络加速</p>
  </td></tr>
  <tr><td style="background:#ffffff;border-radius:14px;border:1px solid #e5e7eb;padding:36px 32px;">
    <table width="100%" cellpadding="0" cellspacing="0">
      <tr><td align="center" style="font-size:34px;padding-bottom:12px;">✅</td></tr>
      <tr><td align="center" style="font-size:22px;font-weight:800;color:#16a34a;padding-bottom:16px;">支付成功</td></tr>
      <tr><td style="font-size:15px;color:#4b5563;line-height:1.7;padding-bottom:18px;">您好，{{.UserName}}，您的订单已支付成功，订阅已自动激活。</td></tr>
      <tr><td style="background:#f9fafb;border-radius:10px;padding:16px 18px;margin-bottom:20px;">
        <table width="100%">
          <tr><td style="font-size:13px;color:#9ca3af;padding-bottom:6px;">订单编号</td><td align="right" style="font-size:13px;color:#374151;font-weight:600;">{{.OrderID}}</td></tr>
          <tr><td style="font-size:13px;color:#9ca3af;">支付金额</td><td align="right" style="font-size:18px;color:#18181b;font-weight:800;">¥{{.Amount}}</td></tr>
        </table>
      </td></tr>
      <tr><td align="center" style="padding-top:8px;">
        <a href="{{.SiteURL}}" style="display:inline-block;background:#4f46e5;color:#ffffff;font-size:14px;font-weight:700;text-decoration:none;padding:12px 32px;border-radius:10px;">前往控制台</a>
      </td></tr>
    </table>
  </td></tr>
  <tr><td align="center" style="padding-top:24px;">
    <a href="{{.SiteURL}}" style="font-size:13px;color:#8b8d98;text-decoration:none;">{{.SiteURL}}</a>
    <p style="font-size:12px;color:#b0b3ba;margin:8px 0 0;">此邮件由 {{.SiteName}} 系统自动发送，请勿直接回复。</p>
  </td></tr>
</table>
</td></tr></table>
</body>
</html>$$,
    updated_at = now()
WHERE name = 'payment_success';

-- 4. 工单回复
UPDATE mail_templates SET
    subject = '{{.SiteName}} - 工单回复通知',
    body = $$<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>工单回复</title></head>
<body style="margin:0;padding:0;background-color:#f3f4f6;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f3f4f6;padding:40px 16px;"><tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
  <tr><td align="center" style="padding-bottom:24px;">
    <span style="font-size:22px;font-weight:800;color:#18181b;">yundu云渡服务</span>
    <p style="margin:4px 0 0;font-size:13px;color:#8b8d98;">稳定 · 高速 · 全球化网络加速</p>
  </td></tr>
  <tr><td style="background:#ffffff;border-radius:14px;border:1px solid #e5e7eb;padding:36px 32px;">
    <table width="100%" cellpadding="0" cellspacing="0">
      <tr><td style="font-size:22px;font-weight:800;color:#18181b;padding-bottom:8px;">您提交的工单有新回复</td></tr>
      <tr><td style="font-size:15px;color:#4b5563;line-height:1.7;padding-bottom:10px;">您好，{{.UserName}}</td></tr>
      <tr><td style="background:#f9fafb;border-left:4px solid #4f46e5;border-radius:8px;padding:14px 18px;margin-bottom:18px;">
        <p style="margin:0 0 6px;font-size:13px;color:#9ca3af;">工单主题</p>
        <p style="margin:0;font-size:15px;color:#18181b;font-weight:600;">{{.TicketSubject}}</p>
      </td></tr>
      <tr><td style="font-size:14px;color:#374151;line-height:1.8;padding:12px 18px;background:#f3f4f6;border-radius:10px;white-space:pre-line;">{{.ReplyContent}}</td></tr>
      <tr><td align="center" style="padding-top:26px;">
        <a href="{{.SiteURL}}" style="display:inline-block;background:#18181b;color:#ffffff;font-size:14px;font-weight:700;text-decoration:none;padding:12px 32px;border-radius:10px;">查看工单</a>
      </td></tr>
    </table>
  </td></tr>
  <tr><td align="center" style="padding-top:24px;">
    <a href="{{.SiteURL}}" style="font-size:13px;color:#8b8d98;text-decoration:none;">{{.SiteURL}}</a>
    <p style="font-size:12px;color:#b0b3ba;margin:8px 0 0;">此邮件由 {{.SiteName}} 系统自动发送，请勿直接回复。</p>
  </td></tr>
</table>
</td></tr></table>
</body>
</html>$$,
    updated_at = now()
WHERE name = 'ticket_reply';

-- 5. 订阅到期
UPDATE mail_templates SET
    subject = '{{.SiteName}} - 订阅到期提醒',
    body = $$<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>订阅到期提醒</title></head>
<body style="margin:0;padding:0;background-color:#f3f4f6;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f3f4f6;padding:40px 16px;"><tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
  <tr><td align="center" style="padding-bottom:24px;">
    <span style="font-size:22px;font-weight:800;color:#18181b;">yundu云渡服务</span>
    <p style="margin:4px 0 0;font-size:13px;color:#8b8d98;">稳定 · 高速 · 全球化网络加速</p>
  </td></tr>
  <tr><td style="background:#ffffff;border-radius:14px;border:1px solid #e5e7eb;padding:36px 32px;">
    <table width="100%" cellpadding="0" cellspacing="0">
      <tr><td style="font-size:22px;font-weight:800;color:#18181b;padding-bottom:8px;">订阅即将到期</td></tr>
      <tr><td style="font-size:15px;color:#4b5563;line-height:1.7;padding-bottom:10px;">您好，{{.UserName}}</td></tr>
      <tr><td style="font-size:15px;color:#4b5563;line-height:1.7;padding-bottom:20px;">您的订阅套餐 <strong style="color:#18181b;">{{.PlanName}}</strong> 将于 <strong style="color:#d97706;">{{.ExpireDate}}</strong> 到期。为避免服务中断，请及时续费。</td></tr>
      <tr><td align="center" style="padding-bottom:8px;">
        <a href="{{.SiteURL}}" style="display:inline-block;background:#d97706;color:#ffffff;font-size:14px;font-weight:700;text-decoration:none;padding:12px 32px;border-radius:10px;">立即续费</a>
      </td></tr>
      <tr><td style="font-size:12px;color:#9ca3af;line-height:1.6;text-align:center;">如您已完成续费，请忽略此邮件。</td></tr>
    </table>
  </td></tr>
  <tr><td align="center" style="padding-top:24px;">
    <a href="{{.SiteURL}}" style="font-size:13px;color:#8b8d98;text-decoration:none;">{{.SiteURL}}</a>
    <p style="font-size:12px;color:#b0b3ba;margin:8px 0 0;">此邮件由 {{.SiteName}} 系统自动发送，请勿直接回复。</p>
  </td></tr>
</table>
</td></tr></table>
</body>
</html>$$,
    updated_at = now()
WHERE name = 'subscription_expired';

-- 6. 流量预警
UPDATE mail_templates SET
    subject = '{{.SiteName}} - 流量使用提醒',
    body = $$<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>流量使用提醒</title></head>
<body style="margin:0;padding:0;background-color:#f3f4f6;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f3f4f6;padding:40px 16px;"><tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
  <tr><td align="center" style="padding-bottom:24px;">
    <span style="font-size:22px;font-weight:800;color:#18181b;">yundu云渡服务</span>
    <p style="margin:4px 0 0;font-size:13px;color:#8b8d98;">稳定 · 高速 · 全球化网络加速</p>
  </td></tr>
  <tr><td style="background:#ffffff;border-radius:14px;border:1px solid #e5e7eb;padding:36px 32px;">
    <table width="100%" cellpadding="0" cellspacing="0">
      <tr><td style="font-size:22px;font-weight:800;color:#18181b;padding-bottom:8px;">流量使用提醒</td></tr>
      <tr><td style="font-size:15px;color:#4b5563;line-height:1.7;padding-bottom:10px;">您好，{{.UserName}}</td></tr>
      <tr><td style="font-size:15px;color:#4b5563;line-height:1.7;padding-bottom:20px;">您的套餐流量已使用 <strong style="color:#dc2626;">{{.TrafficUsed}}</strong> / <strong style="color:#18181b;">{{.TrafficTotal}}</strong>，使用率已达到 80%。</td></tr>
      <tr><td style="background:#f9fafb;border-radius:10px;padding:16px 18px;margin-bottom:20px;">
        <table width="100%">
          <tr><td style="font-size:13px;color:#9ca3af;padding-bottom:6px;">已用流量</td><td align="right" style="font-size:14px;color:#dc2626;font-weight:700;">{{.TrafficUsed}}</td></tr>
          <tr><td style="font-size:13px;color:#9ca3af;">总流量</td><td align="right" style="font-size:14px;color:#18181b;font-weight:700;">{{.TrafficTotal}}</td></tr>
        </table>
      </td></tr>
      <tr><td align="center">
        <a href="{{.SiteURL}}" style="display:inline-block;background:#4f46e5;color:#ffffff;font-size:14px;font-weight:700;text-decoration:none;padding:12px 32px;border-radius:10px;">查看用量 / 升级套餐</a>
      </td></tr>
    </table>
  </td></tr>
  <tr><td align="center" style="padding-top:24px;">
    <a href="{{.SiteURL}}" style="font-size:13px;color:#8b8d98;text-decoration:none;">{{.SiteURL}}</a>
    <p style="font-size:12px;color:#b0b3ba;margin:8px 0 0;">此邮件由 {{.SiteName}} 系统自动发送，请勿直接回复。</p>
  </td></tr>
</table>
</td></tr></table>
</body>
</html>$$,
    updated_at = now()
WHERE name = 'traffic_warning';

-- 7. 注册邮箱验证码
UPDATE mail_templates SET
    subject = '{{.SiteName}} - 邮箱验证码',
    body = $$<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>邮箱验证码</title></head>
<body style="margin:0;padding:0;background-color:#f3f4f6;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;">
<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f3f4f6;padding:40px 16px;"><tr><td align="center">
<table width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;">
  <tr><td align="center" style="padding-bottom:24px;">
    <span style="font-size:22px;font-weight:800;color:#18181b;">yundu云渡服务</span>
    <p style="margin:4px 0 0;font-size:13px;color:#8b8d98;">稳定 · 高速 · 全球化网络加速</p>
  </td></tr>
  <tr><td style="background:#ffffff;border-radius:14px;border:1px solid #e5e7eb;padding:36px 32px;">
    <table width="100%" cellpadding="0" cellspacing="0">
      <tr><td style="font-size:22px;font-weight:800;color:#18181b;padding-bottom:8px;">邮箱验证码</td></tr>
      <tr><td style="font-size:15px;color:#4b5563;line-height:1.7;padding-bottom:10px;">您好，{{.UserName}}</td></tr>
      <tr><td style="font-size:15px;color:#4b5563;line-height:1.7;padding-bottom:24px;">您正在注册 {{.SiteName}}，本次验证码为：</td></tr>
      <tr><td align="center" style="padding-bottom:24px;">
        <span style="display:inline-block;background:#f3f4f6;border:1px dashed #c7c9d1;border-radius:12px;padding:16px 34px;font-size:30px;font-weight:800;letter-spacing:8px;color:#18181b;">{{.Code}}</span>
      </td></tr>
      <tr><td style="font-size:13px;color:#9ca3af;line-height:1.7;">验证码 10 分钟内有效，请勿泄露给他人。如果您没有注册 {{.SiteName}}，请忽略此邮件。</td></tr>
    </table>
  </td></tr>
  <tr><td align="center" style="padding-top:24px;">
    <a href="{{.SiteURL}}" style="font-size:13px;color:#8b8d98;text-decoration:none;">{{.SiteURL}}</a>
    <p style="font-size:12px;color:#b0b3ba;margin:8px 0 0;">此邮件由 {{.SiteName}} 系统自动发送，请勿直接回复。</p>
  </td></tr>
</table>
</td></tr></table>
</body>
</html>$$,
    updated_at = now()
WHERE name = 'verify_code';
