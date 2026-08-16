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

8月16日的问题  支付显示非支付宝码，风险提示同时100打不开

三个症状全部处理完毕，根因都查清了。

修复结果
症状 1：下单显示失败但订单出现在“我的订单” —— 已修复（代码 + 部署）

根因：qiu-pay 挂死期间（22:33-22:38），5 笔支付宝下单调网关超时。代码里本来有“网关失败→取消订单”逻辑，但取消更新复用了请求上下文——epay 15 秒超时期间请求 ctx 已被 api-gateway 超时取消，取消更新静默失败，订单残留 pending（无支付入口的死单），前端 toast“下单失败”但订单已落库
修复：取消改用独立 context.Background()+5s 超时，失败记日志（提交 b99358b，identity-service 已部署 VPS190，备份 .bak.payfix2.*）
数据清理：YunDu 侧 5 笔死单已标记 canceled，qiu-pay 内部对应 5 笔已置过期（余额检测死循环污染同步清除，内部 pending 归零）
症状 2：100.tiktokplay.na.am/admin/orders 打不开 —— 已恢复 + 加自愈

根因：qiu-pay 容器显示 Up 但 HTTP / 路由挂死约 8 小时（API 路由还半活着，所以查单日志还在滚）。这与 08-05 文档记录的故障是同一模式复发
处理：已重启恢复（首页/管理页均 200）；装了看门狗 /opt/yundu/scripts/qiupay-watchdog.sh + 每分钟 cron，探测失败自动 docker restart，5 分钟冷却防重启风暴
症状 3：二维码提示“该内容非支付宝提供，请谨慎使用” —— 结论：YunDu 侧无缺陷，运营侧决策项

查证：qiu-pay 商户 5 配的是静态个人收款码（所有订单返回同一个 qr.alipay.com/fkx16193...），支付宝对个人静态码远程扫码本就有风控提示，这是平台政策
qiu-pay 的支付宝凭证其实是有效的（credential_status=verified，余额查询正常，昨晚 22:19 那单就是靠余额检测自动确认成 paid 的），但 qiu-pay 当前版本只实现了余额查询，不支持当面付动态码（源码里无 precreate）
根治需要运营侧二选一：① 换支持当面付预下单的支付网关（每单生成官方动态码，无警告且免余额检测）② 接受警告提示（支付实际可用）。 YunDu 侧透传 pay_address 的逻辑是对的，无需改动
当前状态：identity-service active、qiu-pay 200、死单 0。支付链路已恢复可用，用户可正常下单扫码（静态码模式，扫码会有支付宝风控提示但能付）。
## 九、尾数调整金额同步修复（2026-08-16，提交 b4273a9）

- **问题**：账单轮询模式下 qiu-pay 对同金额并发订单做尾数调整（6.00→6.01）用于余额检测精确匹配，
  mapi 响应返回调整后 money——但 YunDu 此前未解析该字段：订单仍按 6.00 展示，用户扫码付 6.00，
  qiu-pay 余额检测等待 6.01，金额永远不匹配 → 付款成功订单卡 pending 无法激活。
- **实测验证**：连续两笔 6.00 下单，mapi 分别返回 money="6.00"/"6.01"（第二笔因第一笔 pending 被加尾数）。
- **修复**：
  1. `epay_gateway.go` GatewayPayment 增加 Money 字段，mapi 解析 money（兼容字符串/数字两种形态）；
  2. `createEpayPayment` 以网关返回金额更新 final_amount（仅 pending 状态可改），前端订单详情自动展示调整后应付金额；
  3. user-web 订单详情提示"按应付金额精确到分支付"（静态收款码需用户手动输入金额场景）。
- **已部署**：identity-service（备份 `.bak.amtfix.*`）+ user-web（备份 `7.tiktokplay.na.am.bak.20260816233022`）。
- **运维要点**：该模式下用户手动输错金额（如付 6.00 而非 6.01）仍会导致无法自动确认，
  需在 qiu-pay 后台（100.tiktokplay.na.am）手动核对到账并补单；根治方案仍是换当面付动态码网关。

## 十、余额净差污染事故与人工补单流程（2026-08-17）

- **现象**：订单 P202608162337382967 用户实付 ¥6.01（支付宝已到账），订单卡 expired 未激活。
- **根因**：余额检测按「当前余额 − 下单时基准余额 = 订单金额」匹配，但收款支付宝账户是
  **活跃使用的个人账户**——本单基准 9.09，期间账户另有 ~8 元转出，净差 -1.99，
  6.01 进账被流出对冲，永远无法匹配。加上订单 1 小时过期，双重卡死。
  这是账单轮询模式的根本缺陷：账户任何无关收支都会污染匹配（前夜 2.41 差值残留同因）。
- **人工补单流程（已验证）**：
  1. YunDu DB 把 expired 翻回 pending（`UPDATE payment_orders SET status='pending' WHERE order_no=... AND status='expired'`）；
  2. 用商户密钥计算 MD5 签名，模拟易支付回调 POST `/api/v1/payment/notify/alipay`
     （pid/type/out_trade_no/trade_no/money/trade_status=TRADE_SUCCESS/sign/sign_type），
     走官方链路 MarkPaidIfPending + activateOrder，订阅自动激活；
  3. qiu-pay SQLite 该单置 status=1（paid）。
- **运维建议**：
  1. 收款账户专户专用（绝不混用个人收支），可大幅降低污染概率；
  2. 出现"支付宝已收款但订单不动"→ 按上述补单流程处理（或直接找用户核对金额后补单）；
  3. 根治：换支持当面付动态码的网关，摆脱余额轮询。

## 十一、法币支付渠道池：第三方易支付可随时添加/更换（2026-08-17）

第三方易支付平台稳定性参差、常需更换。支付配置页改为**渠道池**架构：

- **渠道（Channel）**：一套完整可变配置——接口地址、商户ID、MD5密钥、商户私钥、平台公钥、
  接口路径、下单类型等全部字段可变，后台"支付配置 → 法币支付渠道"卡片管理（添加/编辑/删除）。
- **协议**：v1（彩虹标准 MD5：mapi.php/submit.php/api.php）与 v2（api/pay/* 端点，SHA256WithRSA
  签名，兼容 sign_type=MD5 的 V1 端点模式）。V2 支持响应验签+时间戳校验、查单、回调验签。
- **绑定**：支付宝/微信各绑定一个渠道 ID，出问题随时换绑（下拉一选即切，零代码零发版）。
- 换渠道前建议先确保无在途法币订单（旧渠道 pending 订单的查单/回调走新渠道会失效）。
- 旧 alipay/wechat epay 配置自动迁移为默认渠道（未配置渠道池时内存合成）。

**ifz V2 渠道（商户 1034）**：已添加至渠道池（未绑定，当前仍走 qiu-pay）。RSA 签名/商户信息
查询/下单签名均已实测通过；**待运营在 pay.ifz.cc 商户后台把支付域名 `7.tiktokplay.na.am`（用户面板域名）加入
白名单**（平台报"域名没过白"），之后在支付配置页把支付宝绑定切到 ifz 即可启用。
配置与密钥存于本机 `v2版易支付.txt`（已加 .gitignore，勿入库）。

**运维操作（换第三方易支付 SOP）**：
1. 支付配置 → 法币支付渠道 → 添加渠道（填平台提供的接口地址/pid/密钥组，按平台协议选 v1/v2）；
2. 用渠道"编辑"核对配置完整性标记（已配置=密钥组齐全）；
3. 支付宝/微信卡片 → 切换渠道下拉 → 选中新渠道即生效；
4. 旧渠道保留在池中可随时切回，确认废弃后再删除。

## 十二、ifz V2 渠道接入存档（2026-08-17）

### 当前状态：等平台域名审核

- **已完成**：域名 `7.tiktokplay.na.am` 已提交 pay.ifz.cc 商户后台授权支付域名，**平台审核中**。
- **实测留档**：审核通过前下单仍被拦（`该域名不可发起支付，原因：域名没过白`）；
  RSA 签名本身已被平台接受（商户信息查询 code=0，下单到达业务校验层）。

### 渠道完整信息（密钥原件在本机 `v2版易支付.txt`，已 .gitignore 勿入库）

| 项 | 值 |
|---|---|
| 渠道ID | ifz |
| 协议 | v2 / RSA（SHA256WithRSA） |
| 接口地址 | https://pay.ifz.cc |
| 商户ID (pid) | 1034 |
| V1 MD5 密钥 | YllPdx3UrzT0YLPwu73r7UXd3DWZR3p7（txt 内"V1版本"，当前未用） |
| 商户私钥 | txt 内标"商户公钥"的 MIIEvQ… 实为 PKCS#8 私钥（平台生成时的标注误导） |
| 平台公钥 | txt 内 MIIBIjAN…（X.509，用于响应/回调验签） |
| 下单类型 | alipay |
| notify_url | https://7.tiktokplay.na.am/api/v1/payment/notify/alipay |
| return_url | https://7.tiktokplay.na.am/dashboard/orders |
| 生产存储 | system_settings: payment/fiat_channels → channels[id=ifz] |

### 审核通过后的启用步骤（3 步，零代码）

1. 复测下单：`cd apps/identity-service && source <( creds env ) && go test ./internal/service/ -run TestEpayV2LiveCreate -v`
   （ creds 取 v2版易支付.txt 的 EPAYV2_GATEWAY/PID/PRIV/PUB 四个环境变量），返回 `live create OK: trade_no=...` 即通过；
2. 管理面板 → 支付配置 → 支付宝卡片 → 切换渠道 → 选 **ifz V2 易支付**；
3. 用户面板实测一笔小额订单（≥1元，平台最低限额），确认自动到账激活后，qiu-pay 渠道可退役（保留在池中随时切回）。

### 渠道池速查（换任何第三方易支付的 SOP）

```
管理面板 → 支付配置 → 法币支付渠道：
① 添加渠道（v1=彩虹MD5 / v2=RSA，按平台文档填 接口地址/pid/密钥组）
② 状态显示"已配置"= 密钥组齐全
③ 支付宝/微信卡片 → 切换渠道 → 选中新渠道（立即生效，无需重启）
④ 旧渠道保留可切回；换绑前确保无在途法币订单
```

V2 平台特殊注意：支付域名需在平台商户后台授权（域名白名单）；订单最低金额以平台为准（ifz 为 1 元）。
