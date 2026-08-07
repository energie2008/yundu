# Qiu-Pay（彩虹易支付）部署报告

日期：2026-08-05
部署位置：vps190（Docker 容器）
仓库：https://github.com/leoxie2006/qiu-pay （Python/FastAPI/SQLite + Vue3）

---

## 一、部署结果

| 项 | 值 |
|---|---|
| 容器 | qiu-pay（镜像 qiusheng26/qiu-pay:latest），restart=unless-stopped |
| 内部端口 | 127.0.0.1:18000 → 8000（仅本机，经 nginx 反代） |
| 数据目录 | /opt/qiupay/data（SQLite 持久化） |
| 配置目录 | /opt/qiupay/.env（admin 账号/JWT 密钥） |
| 管理后台 | https://100.tiktokplay.na.am （admin / 密码见 /opt/qiupay/.env） |
| 彩虹易支付接口 | https://100.tiktokplay.na.am/xpay/epay/mapi.php（下单 JSON）、/xpay/epay/api.php（查单/商户查询） |
| SSL | 真实 Let's Encrypt 证书（DNS-01，EC-256，acme.sh 自动续期；新 CF token 覆盖 tiktokplay.na.am zone） |
| Cloudflare | 100.tiktokplay.na.am 橙云已开，边缘 TLS 正常 |

## 二、YunDu 面板对接

- 后台支付配置 → 支付宝/微信 → 易支付：
  - 网关地址 `https://100.tiktokplay.na.am/xpay/epay/`
  - 商户 pid `1`、密钥 `558c1b6d50a792ac9435398d91b7a0a8`（qiu-pay 中创建的商户 yundu）
  - 下单类型 alipay / wxpay
- 代码适配（提交 0491060）：
  - QueryOrder 优先 GET api.php?act=order&pid&key&out_trade_no（Qiu-Pay 风格），标准 POST+sign 兜底
  - user-web 订单详情：pay_address 为 http(s) 图片/链接时显示收款码图片而非二维码内容
- 实测：YunDu 支付宝下单成功，返回支付宝收款码 `https://qr.alipay.com/...`；qiu-pay 查单返回订单状态。

## 三、待办（运营侧）

1. **支付宝凭证校验**：qiu-pay 商户 1 的凭证 `credential_status=failed`，
   需在 qiu-pay 后台（https://100.tiktokplay.na.am/v1/admin）重新配置/校验支付宝开放平台凭证，
   否则账单检测无法自动确认到账（收款码已配置可下单，但自动激活依赖凭证有效）。
2. **收款码**：已配置支付宝收款码；如需微信，qiu-pay 当前按 type 出码，确认商户收款码对应类型。
3. **监控**：qiu-pay 容器日志 `docker logs qiu-pay`；数据备份 `/opt/qiupay/data`。

## 四、修复：支付宝/微信订单重复显示两张支付卡

- 现象：CNY(支付宝/微信) 订单经 qiu-pay 也会返回 pay_address（收款码链接），
  原 USDT 支付卡只看 pay_address 导致 USDT 卡与 CNY 卡同时出现（8 USDT / RMB 8 各一张）。
- 修复（提交 4fb49ff）：USDT 支付卡改为仅当订单为 USDT(TRC20/BEP20/EVM) 时渲染；
  CNY 订单只走 CNY 支付卡（显示收款码图片 + 去支付）。
- 实测：支付宝订单 pay_currency=CNY、pay_address=支付宝收款码链接，仅 CNY 卡渲染。

## 五、三处修复（2026-08-05）

1. **100.tiktokplay.na.am 打不开**：qiu-pay 容器 docker 端口转发（127.0.0.1:18000）故障，
   `docker compose restart qiu-pay` 恢复；域名/证书/服务正常。
2. **订单金额币种错乱**（提交 4797d91）：订单详情"商品金额/实付金额"对 CNY(支付宝/微信) 订单
   显示成 USDT（如 $2.08 vs $15.00）。修复：CNY 订单显示人民币（¥），USDT 订单显示 USDT。
3. **续费流量未重置**（提交 4797d91）：同套餐续费原先只延长有效期、保留已用流量。
   修复：续费时延长有效期 + 重置已用流量（新周期重新计算，覆盖上个周期）。

## 六、支付宝收款码显示与去支付按钮修复（提交 99764d1）

- 原因：qiu-pay 返回的 pay_address 是 `qr.alipay.com/...` 收款码**链接**（非图片），
  上一版前端当图片渲染导致空白；"去支付"在浏览器打开该链接会跳到支付宝公告页。
- 修复：非图片 URL 渲染为**二维码**（支付宝/微信扫一扫支付）；"去支付"仅在有真实
  payment_uri 时显示；提示文案改为扫码支付。
- 运维：qiu-pay 上传的收款码图片改为持久化挂载 `/opt/qiupay/data/uploads`
  （此前存在容器内，重启容器会丢）。
