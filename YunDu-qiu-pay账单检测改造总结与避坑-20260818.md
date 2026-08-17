# Qiu-Pay 账单检测改造总结与避坑指南

> 日期：2026-08-18
> 范围：qiu-pay（免签约收款）/ YunDu identity-service / user-web 支付链路
> 状态：账单检测 + 扫码免输金额已上线；两级匹配（check_mode=1）最终版已部署 + S1-S5 单测全绿（见 §4.1）；scheme 已切换 20000123 变体（见 §5.9）；仅剩真实支付端到端验证（见 §6 P0-2）

---

## 1. 改造背景与目标

### 1.1 原方案的问题（余额轮询）

原 qiu-pay 使用**余额检测**确认收款：下单时记录基准余额，轮询对比 `当前余额 - 基准余额 >= 订单金额` 即确认。

缺陷：
- 账户任何无关收支（转出、余额宝自动转入转出、退款、个人消费）都会污染差值 → 乱单
- 多笔同时收款时差值无法区分归属 → 串单/漏单
- 依赖网页 CK（Cookie）扫码登录 → CK 掉线需重扫，运营成本高

### 1.2 目标方案（免 CK 账单检测）

参照成熟方案的「免 CK 模式（PID + 开放平台 RSA 密钥）」：

| 维度 | 旧：余额检测 | 新：账单检测 |
|---|---|---|
| 数据源 | 余额查询接口（差值推断） | 开放平台账单流水 API（逐笔明细） |
| 凭证 | 网页 CK（会掉线） | AppID + 应用私钥 + 支付宝公钥（不掉线） |
| 无关收支影响 | 有（污染差值） | 无（只看入账流水） |
| 并发同收 | 乱单 | 尾数扰动金额唯一，逐笔精确匹配 |
| 局限 | — | 账单接口有 QPS 限流；流水有 1-3 秒延迟 |

配套能力：**扫码免输金额**——下单返回 `alipays://` scheme，支付宝扫码直达转账页、金额自动预填（含扰动尾数），杜绝手输金额错误导致卡单；静态收款码保留为降级。

---

## 2. 支付确认原理（账单检测核心）

1. 开放平台应用开通「账单数据查询及账单下载服务」权限，配置 AppID / 应用私钥 / 支付宝公钥 / 收款 PID（2088 开头）
2. 每笔订单生成**带唯一小数尾数的金额**（如 6.00 → 6.01、6.02），用于区分同金额订单
3. 后端轮询调用 `alipay.data.bill.accountlog.query` 拉取账户收入流水列表
4. 流水与待支付订单匹配（两级匹配，见 §3.3），匹配成功 → 置已支付 + 商户入账 + 回调 YunDu
5. 流水号写入 `orders.api_trade_no`，天然防止同一笔流水被重复消费

**不是余额检测**：不调用余额查询、不靠余额前后相减。退款、余额宝转入转出、无关收支全部不影响匹配。

---

## 3. 改动清单

### 3.1 qiu-pay 侧（补丁目录 `/opt/qiupay/patches/`，compose 只读挂载）

| 文件 | 类型 | 内容 |
|---|---|---|
| `services/bill_checker.py` | 新增 | 账单检测器：拉流水（凭证级 2s 缓存 + 最多 3 页×200 条）、两级匹配、置支付、入账、回调、审计日志（复用 balance_logs 表，前缀 `[账单]`） |
| `services/alipay_client.py` | 修改 | 新增 `query_account_log()` 调用账单流水接口；重构 `_request()` 统一签名/验签 |
| `services/alipay_scheme.py` | 新增 | `build_alipay_transfer_scheme(pid, amount_str, order_no)` 生成 `alipays://platformapi/startapp?appId=20000028&bizData=...` |
| `services/payment_poller.py` | 修改 | 检测入口改 `check_payment_with_fallback`；轮询间隔 1s→2s（适配账单接口 QPS） |
| `routes/query.py` | 修改 | 订单查询触发的检测同样切换到账单检测 |
| `routes/payment.py` | 修改 | mapi 下单响应新增 `alipay_scheme` 字段（严格用金额字符串生成，禁 float） |
| `services/platform_config.py` | 修改 | 凭证保存/读取支持 `alipay_user_id`（收款 PID） |
| `database.py` | 修改 | `merchant_credentials` 表新增 `alipay_user_id` 列（幂等迁移） |
| `services/merchant_service.py` | 修改 | 新增 `delete_merchant()`：有历史订单拒绝删除（防账务丢失），无订单物理删除并级联清理凭证 |
| `routes/admin.py` | 修改 | 新增 `DELETE /v1/admin/merchants/{pid}`；凭证接口接收 `alipay_user_id` |
| SPA `n2-Merchants-*.js` 等 13 个 | 修改 | 管理后台：凭证表单「收款PID」输入框 + 凭证列表「收款PID」列 + 商户「删除」按钮（二次确认）。**注意：是直接改编译后的 minified JS**（无前端源码），全部文件加 `n2-` 前缀 + index.html 换引用以击穿缓存 |

### 3.2 YunDu 侧

| 文件 | 内容 |
|---|---|
| `identity-service/internal/service/epay_gateway.go` | mapi 响应解析 `alipay_scheme`；无跳转 URL 时兜底构造 `/v1/pay/{trade_no}` 收银台链接 |
| `identity-service/internal/service/payment_service.go` | `pay_address` 优先 scheme（静态码降级）；以网关返回金额更新 `final_amount`（6.00→6.01，前端展示与实付一致） |
| `user-web/src/pages/OrderDetail.tsx` | scheme 模式提示「金额已自动填入，请核对后再付款」；轮询查单 |
| `migrations/000078_pay_address_text.sql` | **关键修复**：`payment_orders.pay_address` VARCHAR(64) → TEXT（见 §5.1） |

### 3.3 匹配规则演进（check_mode=1 两级匹配）

| 版本 | 规则 | 状态 |
|---|---|---|
| v1（已上线） | 纯金额匹配 | 已被 v2 取代 |
| v2（已上线） | **第一优先：流水备注含订单号（trade_no / out_trade_no）且金额一致 → 备注匹配**；**第二优先：备注被支付宝丢弃时按金额精确匹配**。同金额改用订单列表支持多笔并发 | S1-S5 单测全绿（2026-08-18，容器内实测部署代码），生产运行中 |

v2 的安全约束：
- 备注命中仍需**金额一致**才确认（防用户手动改小预填金额后凭备注少付过单）
- 同金额多笔待支付时金额映射用**列表**（v1 单值 dict 会在备注匹配消费一笔后，剩余订单无法金额匹配——S5 单测暴露的缺陷）
- 匹配成功即从两级映射移除，防二次消费

---

## 4. 已完成验证

- ✅ 支付宝账单接口调通（HTTP 200，权限已开通）：日志 `账单检测未匹配: 流水=0笔, 待支付=N笔`
- ✅ mapi 下单 E2E：返回 `alipays://...u=2088632230...`（161 字符），扰动金额与 scheme 内金额一致
- ✅ YunDu 下单 E2E（2026-08-18 01:15）：`create-order 201`，`pay_address` 161 字符 scheme 落库，`gateway=epay`，账单检测轮询正常启动
- ✅ PID 已配置：凭证 id=10/11，`alipay_user_id=2088632230249634`
- ✅ 商户删除接口：无订单可删、有订单（yund1 32 笔）被拒
- ✅ 迁移 000078 已应用（DB 版本 78）
- ✅ 管理后台新版 JS 已上线（`n2-` 前缀击穿缓存）
- ✅ 转账 scheme 变体切换上线：`20000123` 收款码协议（V3 形态），真机可扫开且金额自动预填（详见 §5.9）；生产 YunDu 订单实测返回 201 字符新 scheme

### 4.1 2026-08-18 两级匹配最终版部署验证（P0-1 完成）

- ✅ `bill_checker.py` 最终版（金额映射改列表，MD5 `9e38354b`）scp → `/opt/qiupay/patches/`（旧版已备份 `bill_checker.py.bak-20260818-pre-s5`），`docker compose restart qiu-pay` 平滑生效；其余 11 个补丁文件与服务器逐一 MD5 比对一致，无需重发
- ✅ 单测脚本重写：`qp_match_test.py` 旧版第 60 行仍是单值映射残留（`amount_to_order.get` 未定义，S2 即 NameError）——改为 monkeypatch 外部依赖直接驱动**真实** `BillChecker.check_payment` 匹配循环，不再手抄逻辑
- ✅ 容器内 S1-S5 全绿：S1 备注订单号匹配 / S2 金额兜底 / S3 改金额不过单 / S4 同金额备注分流 / S5 混合匹配（列表映射回归，修复确认）
- ✅ 部署后链路：服务 200、启动无报错；mapi E2E 下单返回 scheme（160 字符，PID/金额正确）；账单检测轮询 2-3s 间隔正常、无回退
- ✅ YunDu E2E 下单：`P202608180148454950`（pending / gateway=epay / pay_address=160 字符 scheme / final_amount=6.00），qiu-pay 侧同步建单并轮询
- 运维清理：直连 mapi 的临时验证单（out_trade_no 前缀 `E2E-S5DEP-`）已置过期，防 6.00 金额被无关流水误匹配

---

## 5. 避坑指南（本次实际踩过的坑）

### 5.1 【最高危】DB 字段长度：pay_address VARCHAR(64) 溢出 → 全站下单 500

- **现象**：user-web 下单全部 internal server error；日志显示 `epay adjusted pay amount`（网关调用成功）后 2ms 即 500；订单行 `gateway`/`pay_address` 全空但 `final_amount` 已更新
- **根因**：配置 PID 后 mapi 返回 161 字符 scheme，写入 `VARCHAR(64)` 列报 `value too long`
- **教训**：**凡是引入新字符串格式（scheme/URL），先查 DB 列长度**。ALTER 前 PG 的 varchar 超长是硬错误不截断
- **修复**：goose 迁移 `000078_pay_address_text.sql` 改 TEXT。注意该项目迁移工具要求 `-- +goose Up` / `-- +goose StatementBegin` 标记格式，裸 SQL 会报「未找到有效SQL语句」

### 5.2 金额必须用字符串，严禁 float

- 生成 scheme 的金额**必须**用订单金额字符串（`AmountStr`/`money` 原文），用 float64 会出现 `6.01 → 6.0100000000000005`，账单匹配直接失败
- 匹配计算统一用 `Decimal`，量化到分（`quantize(Decimal("0.01"))`）

### 5.3 浏览器/CF 边缘缓存导致"改了没生效"

- 静态资源响应头 `Cache-Control: max-age=14400`（4 小时），缓存期内 F5 也不回源
- **解法**：改文件名（全量 `n2-` 前缀）+ index.html（不缓存）换引用 → 必然回源
- 长期正解：拿到前端源码后改源码重新构建，不要长期维护 minified JS

### 5.4 两个 PID 是完全不同的东西

| | qiu-pay 商户 PID | 支付宝 PID |
|---|---|---|
| 是什么 | qiu-pay 平台商户 ID（如 5/yund1） | 收款人支付宝账号（2088 开头 16 位） |
| 用途 | YunDu 调 mapi 的身份凭证 | scheme 的收款对象 `u` 参数、账单归属 |
| 存储 | merchants 表 | merchant_credentials.alipay_user_id |

scheme 里 `u` 填错成商户 PID → 支付宝找不到账号，scheme 直接废掉。

### 5.5 公钥/私钥/凭证字段对照

- **私钥**（private_key）：应用私钥，请求签名用
- **公钥**（public_key）：**支付宝公钥**（不是应用公钥！），验签响应用——两把公钥填错导致验签失败
- **AppID**：开放平台应用 ID
- **收款 PID**：2088 开头，选填；不填 = 静态码手输金额模式，填了才启用免输金额

### 5.6 alipays:// scheme 的固有边界

- `m` 备注经常被支付宝丢弃 → 账单备注为空 → 这就是 check_mode=1 必须「订单号优先、金额兜底」的原因
- 微信扫码无法唤起 → 页面必须提示「请使用支付宝 APP 扫码」
- 用户可手动改预填金额 → 金额不符订单不确认，业务层靠订单超时兜底（所以备注匹配也必须校验金额）
- APP 升级可能失效 → 必须保留静态收款码降级按钮

### 5.7 运维操作坑

- **PowerShell 远程执行内嵌 Python**：引号转义地狱，一律本地写 .py → scp → 容器内执行
- **本机无 Python 环境**：语法校验放容器内 `python -m py_compile`
- **迁移工具**：需 `source /opt/yundu/config/.env` 后在 `/opt/yundu` 下执行 `./bin/migrate`（默认 DSN 指向 localhost）
- **测试用户注册**：需邮箱验证码，可直接 Redis 写 `email_code:<email>`（TTL 600s）绕过 SMTP
- **下单接口**：`period_code` 必填，从 `/plans/{id}` 详情取

### 5.8 双侧订单状态会短暂不一致（机制性，非 bug）

YunDu 取消订单不会通知 qiu-pay（彩虹协议无标准关单接口）→ qiu-pay 侧最长滞留 1 小时后 TTL 自动过期。纯展示差异，不影响账务。曾因 §5.1 的 500 放大过该现象（YunDu 落库失败但 qiu-pay 已建单）。

### 5.9 转账 scheme 变体切换（2026-08-18 真机实测）

- ✅ **appId=20000028（转账协议）已被支付宝新版客户端拦截**：扫码报“暂未找到此功能，请稍后再试”（即 §5.6 预警的 APP 升级失效场景，已真机复现）
- ✅ **已切换为收款码协议变体**：`appId=20000123&actionType=scan&biz_data={"s":"money","u":PID,"a":金额,"m":单号}`（biz_data 用 URL 编码，V3 形态）。V2（原文 JSON）/V3（URL 编码）真机均扫开且金额自动预填；旧版已备份 `alipay_scheme.py.bak-20260818-v1`，生产订单实测返回 201 字符新 scheme
- ⚠️ **支付宝风控行为（真机实测）**：扫码付款会多次弹“交易风险”中断，重试数次后仍可支付；带备注与不带备注表现相同（备注长数字单号疑似风控加分项但非决定因素，个人 PID 收款本身在风控观察范围）
- ⚠️ **备注字段 m 在扫码收款场景基本不落账**：实测带/不带备注的码体验一致 → 生产匹配主路径实际是【金额兜底】，备注匹配是“有则更准”的增强。两级模式仍是最优解，无需变更
- 📌 **真金测试教训**：qiu-pay 订单 TTL 仅 1 小时，用户与风控周旋期间订单被 `payment_poller` 置 expired（本次未实际付款无损失）。下次测试：先重新下单再扫码，一气呵成

---

## 6. 下一步改造事项（按优先级）

### P0（立即）

1. ~~**部署两级匹配最终版**~~：✅ 已完成（2026-08-18，见 §4.1）。部署流程：备份旧版 → scp `bill_checker.py` → `docker compose restart qiu-pay` → 容器内跑 `qp_match_test.py`；测试脚本已重写为驱动真实代码路径，后续改动可直接复用
2. **真实支付端到端验证**（唯一未验证的环节）：user-web 下单 → 支付宝真实扫码（金额自动预填）→ 付款 → 观察 qiu-pay 日志 `账单检测匹配成功[金额/备注订单号]` → YunDu 订单自动变已支付 → 套餐激活
   - 2026-08-18 两次尝试：第一次 scheme 被新客户端拦截（已修复，见 §5.9）；第二次用户扫开并进入输密码，但与风控周旋期间订单 TTL 过期，未实际付款。**下次验证：重新下单后立即扫码付款一气呵成**；测试号 `e2e-scheme-20260818@yundu.test` 可复用（脚本 `qp_new_month_order.py` 一键建单）
   - 双级匹配判定依据：日志 `账单检测匹配成功[备注订单号]`（scheme 备注 m 保留时）或 `[金额]`（备注被丢弃时，预期主路径）

### P1（近期）

3. **取消订单同步**：YunDu 取消时调 qiu-pay 关单（或 qiu-pay 提供 close 接口），消除双侧状态不一致窗口；同时避免已取消订单金额被后续流水误匹配
4. **前端源码化**：联系上游拿 qiu-pay 管理后台 Vue 源码，把 minified JS 补丁（收款PID/删除按钮）落回源码，`n2-` 临时方案列入退役计划
5. **扰动尾数复用防撞**：订单过期后尾数循环复用，迟到入账可能误匹配新订单。可加「订单过期后 N 分钟内同金额流水不自动匹配，转人工」或尾数不复用策略

### P2（运营强化）

6. **监控告警**：账单接口失败率/回退余额检测次数（`账单检测失败，回退` 日志关键字）、匹配延迟、QPS 用量
7. **余额检测退役计划**：账单检测稳定运行 2-4 周后，移除回退逻辑与 BalanceChecker，单一数据源
8. **对账脚本**：每日 T+1 对比支付宝账单下载（CSV）与 qiu-pay orders 已支付记录，兜底审计
9. **管理后台审计日志页面化**：balance_logs 里的 `[账单]` 前缀日志目前只能查库，可在后台订单详情展示匹配路径（备注/金额）

---

## 7. 关键命令速查

```bash
# 查看账单检测实时日志
ssh -i "D:\机场搭建\key\190key.pem" root@43.135.147.190 \
  'docker logs qiu-pay --since 5m 2>&1 | grep 账单'

# qiu-pay 补丁部署流程
scp -i key.pem qiupay-src/services/xxx.py root@43.135.147.190:/opt/qiupay/patches/
ssh ... 'cd /opt/qiupay && docker compose restart qiu-pay'

# 两级匹配单测（容器内）
docker cp qp_match_test.py qiu-pay:/tmp/ && docker exec qiu-pay python /tmp/qp_match_test.py

# YunDu 迁移
ssh ... 'set -a; . /opt/yundu/config/.env; set +a; cd /opt/yundu && ./bin/migrate'

# 查 YunDu 订单落库（scheme 长度应为 ~161）
docker exec -i yundu-postgres psql -U app -d airport \
  -c "SELECT order_no,status,LENGTH(pay_address) FROM payment_orders ORDER BY created_at DESC LIMIT 5;"

# qiu-pay 待支付订单
docker exec qiu-pay python -c "import sqlite3;c=sqlite3.connect('/app/data/qiupay.db');print([tuple(r) for r in c.execute('SELECT id,out_trade_no,money,status FROM orders WHERE status=0')])"
```

---

## 8. 环境事实快照（2026-08-18）

- qiu-pay：VPS190 容器 `qiu-pay`，管理后台 `https://100.tiktokplay.na.am`，DB `/app/data/qiupay.db`（挂载持久化）
- 收款凭证：id=10（merchant 5）/ id=11（merchant 6），PID `2088632230249634`，账单权限已开通
- YunDu：identity-service 8081（systemd），PG `yundu-postgres`（app/airport），Redis `yundu-redis`，DB 版本 78
- 商户：yund1（pid=5，32 笔历史订单，禁止删除）、pid=6
- 测试残留：用户 `e2e-scheme-20260818@yundu.test`（可保留作回归，或 admin 后台清理）
