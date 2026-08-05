import { Link } from 'react-router-dom';
import { useTheme } from '../lib/theme';
import { LanguageSelector } from '../components/LanguageSelector';
import { useQuery } from '@tanstack/react-query';
import { api } from '../lib/api';
import { EP, adaptPlan, bytesToGB, getPeriodLabel, type PlanResponse } from '../lib/endpoints';
import {
  Zap,
  Shield,
  Globe,
  Gauge,
  Headphones,
  Moon,
  Sun,
  ArrowRight,
  Check,
  Sparkles,
  UserPlus,
  Mail,
  CreditCard,
  Download,
  Plane,
  Briefcase,
  Film,
  Wifi,
} from 'lucide-react';

function formatTraffic(bytes: number): string {
  if (bytes <= 0) return '不限流量';
  const gb = bytesToGB(bytes);
  return gb >= 1024 ? `${(gb / 1024).toFixed(0)} TB` : `${gb.toFixed(0)} GB`;
}

const features = [
  { icon: Zap, title: '极速连接', desc: '全球优质 BGP 线路，智能路由选优，延迟低至毫秒级' },
  { icon: Shield, title: '安全加密', desc: '端到端 TLS / REALITY 加密，全方位保护您的隐私' },
  { icon: Globe, title: '全球节点', desc: '覆盖亚太、欧美多个地区，持续扩展节点覆盖' },
  { icon: Wifi, title: '多端支持', desc: '支持 pc ios Android等主流客户端' },
  { icon: Gauge, title: '智能负载', desc: '自动负载均衡，高峰期无感切换最优节点' },
  { icon: Headphones, title: '专业支持', desc: '工单系统快速响应，专业团队 7×24 小时为您服务' },
];

const steps = [
  { icon: UserPlus, num: '01', title: '注册账号', desc: '邮箱注册，简单快捷' },
  { icon: Mail, num: '02', title: '邮箱验证', desc: '验证邮箱确保账户安全' },
  { icon: CreditCard, num: '03', title: '充值选购套餐', desc: '多种支付方式，灵活选择' },
  { icon: Download, num: '04', title: '导入配置连接', desc: '一键订阅，极速连接' },
];

const faqs = [
  {
    q: '如何开始使用？',
    a: '注册账号后选择合适的套餐，完成支付即可获得订阅链接，导入支持的客户端即刻使用。',
  },
  {
    q: '支持哪些客户端？',
    a: '支持 pc ios Android等主流客户端',
  },
  {
    q: '流量如何计算？',
    a: '上传与下载流量合并计算，订阅购买日起每 30 天自动重置流量配额。',
  },
  {
    q: '可以退款吗？',
    a: '未使用的套餐支持 7 天无理由退款，请通过工单系统联系客服处理。',
  },
];

const cityCards = [
  {
    img: '/cities/london.jpg',
    city: 'London',
    title: '商务无界',
    caption: '跨洋会议零时差，全球协作触手可及。',
  },
  {
    img: '/cities/maldives.jpg',
    city: 'Maldives',
    title: '旅途自由',
    caption: '身在远方，依旧顺畅访问熟悉的网络世界。',
  },
  {
    img: '/cities/hongkong.jpg',
    city: 'Hong Kong',
    title: '流媒体无界',
    caption: '4K 高清与海外好剧，一键接入随时畅看。',
  },
  {
    img: '/cities/helicity.jpg',
    city: 'Global',
    title: '随心起飞',
    caption: '下一站任意国度，自由网络始终同行。',
  },
];

/**
 * 参考"轩辕"获奖设计的玻璃地球
 * - 薄荷青/青绿玻璃质感球体，与参考图一致
 * - 大陆是柔和青绿块（不是描边轮廓）
 * - 经纬线为同色系细线
 * - 青蓝色光点（带脉冲）+ 弧线互联
 * - 绕垂直轴（Y轴）左右旋转——地球自转方向
 * - 背景：白→浅薄荷青渐变
 */
function RealisticGlobe() {
  // 国际大都市节点（lon, lat, name）
  const cities = [
    { lon: 116.4, lat: 39.9 },   // 北京
    { lon: 139.7, lat: 35.7 },   // 东京
    { lon: 103.8, lat: 1.35 },   // 新加坡
    { lon: 55.3, lat: 25.3 },    // 迪拜
    { lon: 2.35, lat: 48.9 },    // 巴黎
    { lon: -0.13, lat: 51.5 },   // 伦敦
    { lon: -74.0, lat: 40.7 },   // 纽约
    { lon: -118.2, lat: 34.0 },  // 洛杉矶
    { lon: -46.6, lat: -23.5 },  // 圣保罗
    { lon: 151.2, lat: -33.9 },  // 悉尼
    { lon: 37.6, lat: 55.8 },    // 莫斯科
    { lon: 77.2, lat: 28.6 },    // 新德里
  ];

  // 大陆路径（柔和青绿块，更简洁精致）
  const landPaths: Array<{ d: string; fill: string }> = [
    { d: 'M150,200 Q220,160 320,175 Q380,195 400,260 Q415,320 380,370 Q320,410 270,395 Q200,380 180,330 Q140,270 150,200 Z', fill: 'url(#contFill)' },
    { d: 'M230,380 Q280,390 305,430 Q280,480 230,475 Q185,450 200,410 Z', fill: 'url(#contFill)' },
    { d: 'M330,120 Q370,110 390,140 Q380,170 350,175 Q320,165 330,120 Z', fill: 'url(#contFill)' },
    { d: 'M280,490 Q340,475 365,530 Q380,610 350,700 Q320,750 285,760 Q255,730 260,650 Q265,570 280,490 Z', fill: 'url(#contFill)' },
    { d: 'M470,200 Q520,185 560,198 Q580,220 565,255 Q540,280 510,282 Q480,270 470,245 Q462,220 470,200 Z', fill: 'url(#contFill)' },
    { d: 'M495,290 Q560,275 595,320 Q620,400 595,490 Q565,560 525,580 Q485,555 475,480 Q468,380 495,290 Z', fill: 'url(#contFill)' },
    { d: 'M580,160 Q720,130 860,180 Q930,230 915,320 Q880,400 800,425 Q700,440 620,400 Q570,360 560,295 Q555,225 580,160 Z', fill: 'url(#contFill)' },
    { d: 'M680,400 Q740,395 770,440 Q740,485 690,480 Q660,450 680,400 Z', fill: 'url(#contFill)' },
    { d: 'M780,430 Q830,420 845,460 Q820,495 785,485 Q765,460 780,430 Z', fill: 'url(#contFill)' },
    { d: 'M880,250 Q895,240 900,265 Q892,290 880,285 Q870,268 880,250 Z', fill: 'url(#contFill)' },
    { d: 'M790,495 Q850,490 870,510 Q830,520 790,515 Z', fill: 'url(#contFill)' },
    { d: 'M790,600 Q860,585 895,620 Q900,665 855,685 Q800,675 780,645 Q775,615 790,600 Z', fill: 'url(#contFill)' },
  ];

  const longitudes = Array.from({ length: 16 }, (_, i) => (i / 15) * 2000);
  const latitudes = [-60, -30, 0, 30, 60];

  const projectWide = (lon: number, lat: number) => ({
    x: 500 + (lon / 180) * 440,
    y: 470 - (lat / 90) * 414,
  });

  // 城市间连线（索引对，弧线连接）
  const links = [
    [0, 1], [0, 2], [1, 2], [2, 3], [3, 4], [4, 5],
    [5, 6], [6, 7], [7, 8], [3, 11], [11, 0], [2, 9], [6, 10], [10, 5],
  ];

  const GLOBE_SIZE = 740; // 地球直径（比 640 大约 15%）

  return (
    <div className="absolute inset-0 overflow-hidden pointer-events-none" aria-hidden="true">
      {/* 背景光晕（加强） */}
      <div
        className="absolute"
        style={{
          right: '-5%',
          top: '0%',
          width: '65%',
          height: '80%',
          background: 'radial-gradient(ellipse at center, rgba(59,169,156,0.18) 0%, rgba(59,169,156,0.07) 35%, rgba(59,169,156,0.02) 60%, transparent 80%)',
          filter: 'blur(40px)',
        }}
      />

      {/* 地球容器：中间偏右，顶部略被 header 盖一小部分 */}
      <div
        className="globe-wrap absolute"
        style={{
          right: '8%',
          top: '-40px',
          width: `${GLOBE_SIZE}px`,
          height: `${GLOBE_SIZE}px`,
          zIndex: 1,
        }}
      >
        {/* 外层大气辉光（更亮更广） */}
        <div
          className="absolute rounded-full"
          style={{
            inset: '-80px',
            background: 'radial-gradient(circle, rgba(59,169,156,0.28) 0%, rgba(59,169,156,0.12) 35%, rgba(59,169,156,0.05) 55%, transparent 75%)',
            filter: 'blur(30px)',
          }}
        />
        {/* 外层第二层辉光 */}
        <div
          className="absolute rounded-full"
          style={{
            inset: '-40px',
            background: 'radial-gradient(circle, rgba(92,201,188,0.20) 0%, rgba(92,201,188,0.06) 40%, transparent 70%)',
            filter: 'blur(18px)',
          }}
        />

        {/* 细轨道环 1 — 平面外圆环（非常淡） */}
        <div
          className="absolute rounded-full pointer-events-none"
          style={{
            inset: '-18px',
            border: '1px solid rgba(59,169,156,0.12)',
          }}
        />

        {/* === 球体本体 === */}
        <div
          className="absolute inset-0 rounded-full"
          style={{
            background: 'radial-gradient(circle at 32% 28%, #f7fffd 0%, #ecf8f5 15%, #d8ece7 35%, #bdd9d2 55%, #9bbfb5 75%, #7ca79c 95%)',
            boxShadow:
              'inset -25px -35px 70px rgba(45,95,88,0.30), inset 18px 24px 50px rgba(255,255,255,0.55), 0 25px 70px -15px rgba(59,169,156,0.25)',
          }}
        >
          {/* 地图层（圆裁切 + 水平滚动自转） */}
          <div
            className="absolute inset-0 rounded-full overflow-hidden"
            style={{
              maskImage: 'radial-gradient(circle at 50% 50%, #000 58%, rgba(0,0,0,0.35) 80%, transparent 100%)',
              WebkitMaskImage: 'radial-gradient(circle at 50% 50%, #000 58%, rgba(0,0,0,0.35) 80%, transparent 100%)',
            }}
          >
            <div
              className="absolute"
              style={{
                top: 0,
                left: '-50%',
                width: '200%',
                height: '100%',
                animation: 'spinMap 50s linear infinite',
              }}
            >
              <svg viewBox="0 0 2000 1000" width="2000" height="1000" preserveAspectRatio="none" style={{ width: '100%', height: '100%' }}>
                <defs>
                  <radialGradient id="contFill" cx="50%" cy="50%" r="50%">
                    <stop offset="0%" stopColor="rgba(59,169,156,0.28)" />
                    <stop offset="100%" stopColor="rgba(59,169,156,0.12)" />
                  </radialGradient>
                  <filter id="blurSoft"><feGaussianBlur stdDeviation="4" /></filter>
                  <linearGradient id="arcGrad" x1="0%" y1="0%" x2="100%" y2="0%">
                    <stop offset="0%" stopColor="rgba(59,169,156,0)" />
                    <stop offset="50%" stopColor="rgba(92,201,188,0.55)" />
                    <stop offset="100%" stopColor="rgba(59,169,156,0)" />
                  </linearGradient>
                </defs>

                {/* 经纬线（更稀、更淡） */}
                <g stroke="rgba(59,169,156,0.10)" strokeWidth="0.5" fill="none">
                  {longitudes.map((x, i) => (
                    <line key={`mlon-${i}`} x1={x} y1="0" x2={x} y2="1000" />
                  ))}
                  {latitudes.map((lat, i) => {
                    const y = 500 - (lat / 90) * 440;
                    return <line key={`mlat-${i}`} x1="0" y1={y} x2="2000" y2={y} />;
                  })}
                </g>

                {/* 大陆 */}
                {[0, 1000].map((off, k) => (
                  <g key={`land-${k}`} transform={`translate(${off},0)`} filter="url(#blurSoft)">
                    {landPaths.map((p, i) => (
                      <path key={`lp-${i}`} d={p.d} fill={p.fill} />
                    ))}
                  </g>
                ))}

                {/* 城市节点（两组） */}
                {[0, 1000].map((off, k) => (
                  <g key={`pts-${k}`}>
                    {cities.map((c, i) => {
                      const p = projectWide(c.lon, c.lat);
                      const x = p.x + off;
                      const y = p.y;
                      const delay = (i * 0.35) % 3;
                      return (
                        <g key={`city-${k}-${i}`}>
                          {/* 外层柔和光晕 */}
                          <circle cx={x} cy={y} r="12" fill="rgba(92,201,188,0.15)" />
                          <circle cx={x} cy={y} r="6" fill="rgba(92,201,188,0.22)" />
                          {/* 脉冲环（双层、更亮更大） */}
                          <circle cx={x} cy={y} r="3" fill="none" stroke="rgba(92,201,188,0.7)" strokeWidth="1.2">
                            <animate attributeName="r" values="3;18;3" dur="2.4s" repeatCount="indefinite" begin={`${delay}s`} />
                            <animate attributeName="opacity" values="0.9;0;0.9" dur="2.4s" repeatCount="indefinite" begin={`${delay}s`} />
                          </circle>
                          <circle cx={x} cy={y} r="3" fill="none" stroke="rgba(255,255,255,0.5)" strokeWidth="0.8">
                            <animate attributeName="r" values="3;12;3" dur="2.4s" repeatCount="indefinite" begin={`${delay + 0.4}s`} />
                            <animate attributeName="opacity" values="0.8;0;0.8" dur="2.4s" repeatCount="indefinite" begin={`${delay + 0.4}s`} />
                          </circle>
                          {/* 实心核心点（更亮） */}
                          <circle cx={x} cy={y} r="3" fill="#5CC9BC" />
                          <circle cx={x} cy={y} r="1.4" fill="#ffffff" />
                        </g>
                      );
                    })}
                  </g>
                ))}
              </svg>
            </div>
          </div>

          {/* 顶部高光（玻璃反射 - 更克制） */}
          <div
            className="absolute inset-0 rounded-full pointer-events-none"
            style={{
              background:
                'radial-gradient(ellipse 34% 26% at 30% 22%, rgba(255,255,255,0.65) 0%, rgba(255,255,255,0.18) 40%, transparent 68%)',
              mixBlendMode: 'screen',
            }}
          />
          {/* 右下暗角 */}
          <div
            className="absolute inset-0 rounded-full pointer-events-none"
            style={{
              background:
                'radial-gradient(ellipse 45% 40% at 78% 80%, rgba(35,78,72,0.28) 0%, rgba(35,78,72,0.06) 42%, transparent 65%)',
            }}
          />
          {/* 边缘 1px 柔光边 */}
          <div
            className="absolute inset-0 rounded-full pointer-events-none"
            style={{ boxShadow: 'inset 0 0 0 1px rgba(255,255,255,0.35), inset 0 0 0 1px rgba(59,169,156,0.15)' }}
          />
        </div>

        {/* === 球体外围的现代弧线连线 + 独立光点 + 倾斜轨道环 === */}
        <svg
          className="absolute pointer-events-none"
          viewBox={`0 0 ${GLOBE_SIZE} ${GLOBE_SIZE}`}
          width={GLOBE_SIZE}
          height={GLOBE_SIZE}
          style={{ overflow: 'visible' }}
        >
          <defs>
            <linearGradient id="flyLineGrad" x1="0%" y1="0%" x2="100%" y2="0%">
              <stop offset="0%" stopColor="rgba(92,201,188,0)" />
              <stop offset="50%" stopColor="rgba(92,201,188,0.8)" />
              <stop offset="100%" stopColor="rgba(92,201,188,0)" />
            </linearGradient>
            <radialGradient id="outerDot">
              <stop offset="0%" stopColor="rgba(92,201,188,0.9)" />
              <stop offset="60%" stopColor="rgba(92,201,188,0.2)" />
              <stop offset="100%" stopColor="rgba(92,201,188,0)" />
            </radialGradient>
            <linearGradient id="ringGrad" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stopColor="rgba(59,169,156,0)" />
              <stop offset="50%" stopColor="rgba(59,169,156,0.22)" />
              <stop offset="100%" stopColor="rgba(59,169,156,0)" />
            </linearGradient>
            <filter id="outerBlur" x="-50%" y="-50%" width="200%" height="200%">
              <feGaussianBlur stdDeviation="3" />
            </filter>
          </defs>

          {/* 倾斜椭圆轨道环 1（透视扁平 + 渐变描边 + 自转） */}
          <g style={{ transformOrigin: '50% 50%', animation: 'ringTiltSpin 120s linear infinite' }}>
            <ellipse
              cx={GLOBE_SIZE / 2}
              cy={GLOBE_SIZE / 2}
              rx={GLOBE_SIZE / 2 + 30}
              ry={(GLOBE_SIZE / 2 + 30) * 0.34}
              fill="none"
              stroke="url(#ringGrad)"
              strokeWidth="1"
            />
          </g>
          {/* 倾斜椭圆轨道环 2（虚线、反向、更扁更大） */}
          <g style={{ transformOrigin: '50% 50%', animation: 'ringTiltSpin2 160s linear infinite reverse' }}>
            <ellipse
              cx={GLOBE_SIZE / 2}
              cy={GLOBE_SIZE / 2}
              rx={GLOBE_SIZE / 2 + 42}
              ry={(GLOBE_SIZE / 2 + 42) * 0.28}
              fill="none"
              stroke="rgba(59,169,156,0.10)"
              strokeWidth="1"
              strokeDasharray="3 5"
            />
          </g>

          {/* 独立漂浮光点（更多更亮） */}
          {[
            { x: 640, y: 90, d: 0, s: 1.2 },
            { x: 100, y: 150, d: 0.8, s: 1.0 },
            { x: 680, y: 580, d: 1.6, s: 0.9 },
            { x: 50, y: 500, d: 0.3, s: 1.0 },
            { x: 380, y: 30, d: 2.0, s: 0.7 },
            { x: 700, y: 340, d: 1.2, s: 0.8 },
            { x: 30, y: 320, d: 2.5, s: 0.7 },
          ].map((dot, i) => (
            <g key={`od-${i}`}>
              <circle cx={dot.x} cy={dot.y} r={14 * dot.s} fill="url(#outerDot)">
                <animate attributeName="opacity" values="0.5;1;0.5" dur="2.4s" begin={`${dot.d}s`} repeatCount="indefinite" />
              </circle>
              <circle cx={dot.x} cy={dot.y} r={8 * dot.s} fill="rgba(92,201,188,0.3)" />
              <circle cx={dot.x} cy={dot.y} r={3.5 * dot.s} fill="#5CC9BC" />
              <circle cx={dot.x} cy={dot.y} r={1.4 * dot.s} fill="#fff" />
            </g>
          ))}

          {/* 多条贝塞尔弧线航线 */}
          {[
            { d: `M 640 90 Q ${GLOBE_SIZE * 0.85} ${GLOBE_SIZE * 0.12} ${GLOBE_SIZE * 0.72} ${GLOBE_SIZE * 0.30}`, o: 0.6 },
            { d: `M 100 150 Q ${GLOBE_SIZE * 0.22} ${GLOBE_SIZE * 0.18} ${GLOBE_SIZE * 0.35} ${GLOBE_SIZE * 0.32}`, o: 0.5 },
            { d: `M 680 580 Q ${GLOBE_SIZE * 0.82} ${GLOBE_SIZE * 0.78} ${GLOBE_SIZE * 0.65} ${GLOBE_SIZE * 0.65}`, o: 0.45 },
            { d: `M 50 500 Q ${GLOBE_SIZE * 0.20} ${GLOBE_SIZE * 0.72} ${GLOBE_SIZE * 0.35} ${GLOBE_SIZE * 0.62}`, o: 0.4 },
            { d: `M 380 30 Q ${GLOBE_SIZE * 0.55} ${GLOBE_SIZE * -0.05} ${GLOBE_SIZE * 0.55} ${GLOBE_SIZE * 0.20}`, o: 0.4 },
          ].map((arc, i) => (
            <path
              key={`arc-${i}`}
              d={arc.d}
              stroke="url(#flyLineGrad)"
              strokeWidth="1.2"
              fill="none"
              strokeDasharray="3 4"
              opacity={arc.o}
            />
          ))}

          {/* 流光盘：沿弧线跑的小光点（多组，更快更强） */}
          {[
            { path: `M 640 90 Q ${GLOBE_SIZE * 0.85} ${GLOBE_SIZE * 0.12} ${GLOBE_SIZE * 0.72} ${GLOBE_SIZE * 0.30}`, dur: 2.2, delay: 0, size: 3.5, whiteSize: 2 },
            { path: `M 100 150 Q ${GLOBE_SIZE * 0.22} ${GLOBE_SIZE * 0.18} ${GLOBE_SIZE * 0.35} ${GLOBE_SIZE * 0.32}`, dur: 2.8, delay: 0.6, size: 3, whiteSize: 1.8 },
            { path: `M 680 580 Q ${GLOBE_SIZE * 0.82} ${GLOBE_SIZE * 0.78} ${GLOBE_SIZE * 0.65} ${GLOBE_SIZE * 0.65}`, dur: 3, delay: 1.2, size: 2.8, whiteSize: 1.6 },
            { path: `M 50 500 Q ${GLOBE_SIZE * 0.20} ${GLOBE_SIZE * 0.72} ${GLOBE_SIZE * 0.35} ${GLOBE_SIZE * 0.62}`, dur: 3.2, delay: 1.8, size: 2.8, whiteSize: 1.6 },
            { path: `M 380 30 Q ${GLOBE_SIZE * 0.55} ${GLOBE_SIZE * -0.05} ${GLOBE_SIZE * 0.55} ${GLOBE_SIZE * 0.20}`, dur: 2.5, delay: 0.3, size: 2.5, whiteSize: 1.4 },
            // 反向跑的光
            { path: `M ${GLOBE_SIZE * 0.72} ${GLOBE_SIZE * 0.30} Q ${GLOBE_SIZE * 0.85} ${GLOBE_SIZE * 0.12} 640 90`, dur: 2.5, delay: 1.1, size: 2.5, whiteSize: 1.4 },
            { path: `M ${GLOBE_SIZE * 0.35} ${GLOBE_SIZE * 0.32} Q ${GLOBE_SIZE * 0.22} ${GLOBE_SIZE * 0.18} 100 150`, dur: 3, delay: 2.0, size: 2.5, whiteSize: 1.4 },
          ].map((fly, i) => (
            <g key={`fly-${i}`}>
              <circle r={fly.size} fill="#5CC9BC" filter="url(#outerBlur)">
                <animateMotion dur={`${fly.dur}s`} begin={`${fly.delay}s`} repeatCount="indefinite" path={fly.path} />
              </circle>
              <circle r={fly.whiteSize} fill="#ffffff">
                <animateMotion dur={`${fly.dur}s`} begin={`${fly.delay}s`} repeatCount="indefinite" path={fly.path} />
              </circle>
            </g>
          ))}
        </svg>

        {/* 外环轨道移动光点（更亮、更快） */}
        <div
          className="absolute pointer-events-none"
          style={{ inset: '-18px', animation: 'ringYSpin 40s linear infinite' }}
        >
          <div
            className="absolute rounded-full"
            style={{
              width: '8px',
              height: '8px',
              top: '50%',
              right: '-4px',
              transform: 'translateY(-50%)',
              background: 'radial-gradient(circle, #fff 0%, #5CC9BC 50%, transparent 100%)',
              boxShadow: '0 0 14px 4px rgba(92,201,188,0.7)',
            }}
          />
        </div>
        {/* 第二颗轨道光点（反向、虚线上） */}
        <div
          className="absolute pointer-events-none"
          style={{ inset: '-30px', animation: 'ringYSpinRev 55s linear infinite' }}
        >
          <div
            className="absolute rounded-full"
            style={{
              width: '6px',
              height: '6px',
              top: '50%',
              left: '-3px',
              transform: 'translateY(-50%)',
              background: 'radial-gradient(circle, #fff 0%, #7FE0D5 60%, transparent 100%)',
              boxShadow: '0 0 12px 3px rgba(127,224,213,0.6)',
            }}
          />
        </div>
      </div>

      {/* 左侧文字区渐变遮罩：减弱，让地球从文字下方透出 */}
      <div
        className="absolute inset-y-0 left-0 pointer-events-none"
        style={{
          width: '65%',
          background:
            'linear-gradient(to right, #ffffff 0%, rgba(255,255,255,0.92) 30%, rgba(255,255,255,0.6) 55%, rgba(255,255,255,0.2) 78%, transparent 100%)',
          zIndex: 1,
        }}
      />
      {/* 顶部融合 header */}
      <div
        className="absolute inset-x-0 top-0 pointer-events-none"
        style={{
          height: '100px',
          background: 'linear-gradient(to bottom, #ffffff 0%, rgba(255,255,255,0.7) 55%, transparent 100%)',
          zIndex: 2,
        }}
      />
      {/* 底部融合下一区 */}
      <div
        className="absolute inset-x-0 bottom-0 pointer-events-none"
        style={{
          height: '70px',
          background: 'linear-gradient(to top, #ffffff 0%, transparent 100%)',
        }}
      />

      <style>{`
        /* 自转方向：向右（与之前相反），速度 50s 一圈（更快） */
        @keyframes spinMap {
          0%   { transform: translateX(-50%); }
          100% { transform: translateX(0); }
        }
        @keyframes ringYSpin {
          from { transform: rotate(0deg); }
          to   { transform: rotate(360deg); }
        }
        @keyframes ringYSpinRev {
          from { transform: rotate(360deg); }
          to   { transform: rotate(0deg); }
        }
        @keyframes ringTiltSpin {
          from { transform: rotate(0deg); }
          to   { transform: rotate(360deg); }
        }
        @keyframes ringTiltSpin2 {
          from { transform: rotate(0deg); }
          to   { transform: rotate(-360deg); }
        }
        /* ====== 移动端地球适配 ====== */
        @media (max-width: 900px) {
          .globe-wrap {
            right: -18% !important;
            top: 5% !important;
            transform: scale(0.72) !important;
            transform-origin: right center;
            opacity: 0.75;
          }
        }
        @media (max-width: 640px) {
          .globe-wrap {
            right: -35% !important;
            top: 12% !important;
            transform: scale(0.55) !important;
            transform-origin: right center;
            opacity: 0.6;
          }
        }
      `}</style>
    </div>
  );
}

function PlanCard({ plan }: { plan: PlanResponse }) {
  const monthPrice = plan.prices?.find(p => p.period_code === 'month' || p.period_code === 'monthly');
  const defaultPrice = monthPrice || plan.prices?.[0];
  const priceCNY = defaultPrice?.price_cny || 0;
  const featureList = plan.features?.slice(0, 4) || [];
  const isPopular = plan.prices && plan.prices.length > 0 && priceCNY >= 30 && priceCNY <= 80;

  return (
    <div
      className={`relative flex flex-col rounded-2xl p-7 transition-all duration-300 ${
        isPopular
          ? 'bg-[var(--card)] shadow-xl scale-[1.02]'
          : 'bg-[var(--card)] border border-[var(--border)] hover:shadow-lg hover:shadow-teal-500/5 hover:-translate-y-0.5'
      }`}
      style={
        isPopular
          ? { boxShadow: '0 20px 50px -12px rgba(59,169,156,0.25)', border: '2px solid var(--primary)' }
          : undefined
      }
    >
      {isPopular && (
        <div className="absolute -top-3 left-1/2 -translate-x-1/2">
          <span
            className="inline-flex items-center gap-1 px-4 py-1 rounded-full text-xs font-medium text-white"
            style={{
              background: 'linear-gradient(135deg, var(--primary) 0%, var(--primary-soft) 100%)',
              boxShadow: '0 6px 16px -4px rgba(59,169,156,0.4)',
            }}
          >
            <Sparkles className="w-3 h-3" />
            最受欢迎
          </span>
        </div>
      )}

      <h3 className="text-lg font-semibold text-[var(--foreground)] mb-1">{plan.name}</h3>
      <p className="text-sm font-medium mb-4" style={{ color: 'var(--secondary-foreground)' }}>
        {formatTraffic(plan.traffic_bytes)} · 购买日起每 30 天重置
      </p>

      <div className="mb-6">
        <span className="text-4xl font-bold text-[var(--primary)]">¥{priceCNY.toFixed(0)}</span>
        <span className="text-[var(--secondary-foreground)] text-sm ml-1 font-medium">
          /{defaultPrice ? getPeriodLabel(defaultPrice.period_code) : '月'}
        </span>
      </div>

      <div className="space-y-2.5 mb-8 flex-1">
        {featureList.length > 0 ? (
          featureList.map((feat, i) => {
            const clean = feat.replace(/^[✅✔️✓☑️\s]+/, '').trim();
            return (
              <div key={i} className="flex items-start gap-2.5 text-sm">
                <div className="mt-0.5 w-4 h-4 rounded-full bg-[var(--accent)] flex items-center justify-center flex-shrink-0">
                  <Check className="w-2.5 h-2.5 text-[var(--accent-foreground)]" strokeWidth={3} />
                </div>
                <span className="text-[var(--foreground)]">{clean || feat}</span>
              </div>
            );
          })
        ) : (
          <>
            {plan.speed_limit_mbps && plan.speed_limit_mbps > 0 && (
              <div className="flex items-start gap-2.5 text-sm">
                <div className="mt-0.5 w-4 h-4 rounded-full bg-[var(--accent)] flex items-center justify-center flex-shrink-0">
                  <Check className="w-2.5 h-2.5 text-[var(--accent-foreground)]" strokeWidth={3} />
                </div>
                <span>{plan.speed_limit_mbps}Mbps 网络速率</span>
              </div>
            )}
            <div className="flex items-start gap-2.5 text-sm">
              <div className="mt-0.5 w-4 h-4 rounded-full bg-[var(--accent)] flex items-center justify-center flex-shrink-0">
                <Check className="w-2.5 h-2.5 text-[var(--accent-foreground)]" strokeWidth={3} />
              </div>
              <span>全线路流媒体解锁</span>
            </div>
            <div className="flex items-start gap-2.5 text-sm">
              <div className="mt-0.5 w-4 h-4 rounded-full bg-[var(--accent)] flex items-center justify-center flex-shrink-0">
                <Check className="w-2.5 h-2.5 text-[var(--accent-foreground)]" strokeWidth={3} />
              </div>
              <span>{plan.device_limit && plan.device_limit > 0 ? `最多 ${plan.device_limit} 台设备` : '不限制设备数量'}</span>
            </div>
          </>
        )}
      </div>

      <Link
        to="/register"
        className={`block w-full py-3 rounded-xl text-center text-sm font-medium transition-all duration-200 ${
          isPopular
            ? 'text-white hover:-translate-y-0.5'
            : 'bg-[var(--muted)] text-[var(--foreground)] hover:bg-[var(--primary)] hover:text-white'
        }`}
        style={
          isPopular
            ? {
                background: 'linear-gradient(135deg, var(--primary) 0%, var(--primary-soft) 100%)',
                boxShadow: '0 8px 20px -6px rgba(59,169,156,0.4)',
              }
            : undefined
        }
      >
        立即选购
        <ArrowRight className="inline w-4 h-4 ml-1" />
      </Link>
    </div>
  );
}

export function Landing() {
  const { theme, toggleTheme } = useTheme();

  const plansQuery = useQuery<PlanResponse[]>({
    queryKey: ['landing-plans'],
    queryFn: async () => {
      try {
        const data = await api.get<PlanResponse[]>(EP.PLANS_GUEST);
        return (data || []).filter(p => p.status === 'active').map(adaptPlan).slice(0, 3);
      } catch {
        return [];
      }
    },
    staleTime: 5 * 60 * 1000,
  });

  const plans = plansQuery.data || [];

  const fallbackPlans: PlanResponse[] = [
    {
      id: '1', code: 'light', name: '轻量套餐', description: '', content: '',
      status: 'active', billing_type: 'recurring', traffic_bytes: 66 * 1024 * 1024 * 1024,
      speed_limit_mbps: 1000, device_limit: 0, reset_cycle: 'monthly',
      features: ['1Gbps 网络速率', '全线路流媒体解锁', 'BGP 三网优化', '不限制设备数量'],
      feature_flags: {}, prices: [{ period_code: 'month', price_usdt: 1, price_cny: 6 }],
      node_count: 12, created_at: new Date().toISOString(),
    },
    {
      id: '2', code: 'pro', name: '增强套餐', description: '', content: '',
      status: 'active', billing_type: 'recurring', traffic_bytes: 156 * 1024 * 1024 * 1024,
      speed_limit_mbps: 1200, device_limit: 0, reset_cycle: 'monthly',
      features: ['1.2Gbps 网络速率', '全线路流媒体解锁+原生IP', 'BGP 智能路由', '更多地区节点'],
      feature_flags: {}, prices: [{ period_code: 'month', price_usdt: 6, price_cny: 43 }],
      node_count: 25, created_at: new Date().toISOString(),
    },
    {
      id: '3', code: 'luxury', name: '轻奢套餐', description: '', content: '',
      status: 'active', billing_type: 'recurring', traffic_bytes: 0,
      speed_limit_mbps: 2000, device_limit: 0, reset_cycle: 'monthly',
      features: ['2Gbps 网络速率', '解锁全部地区', 'BGP 三网优化+专线', '包含轻量+增强节点'],
      feature_flags: {}, prices: [{ period_code: 'month', price_usdt: 25, price_cny: 180 }],
      node_count: 35, created_at: new Date().toISOString(),
    },
  ];

  const displayPlans = plans.length > 0 ? plans : fallbackPlans;

  return (
    <div
      className="min-h-screen"
      style={{
        background: '#ffffff',
        // 页面级主题覆盖：薄荷青主色（与地球呼应）
        ['--primary' as any]: '#3ba99c',
        ['--primary-soft' as any]: '#5CC9BC',
        ['--primary-foreground' as any]: '#ffffff',
        ['--background' as any]: '#ffffff',
        ['--foreground' as any]: '#0f2a26',
        ['--card' as any]: '#ffffff',
        ['--card-foreground' as any]: '#0f2a26',
        ['--muted' as any]: '#f2f8f6',
        ['--muted-foreground' as any]: '#5f847d',
        ['--accent' as any]: '#e0f2ee',
        ['--accent-foreground' as any]: '#3ba99c',
        ['--secondary' as any]: '#f2f8f6',
        ['--secondary-foreground' as any]: '#3d6b64',
        ['--border' as any]: '#e0eeea',
        ['--header-bg' as any]: 'rgba(255,255,255,0.82)',
        ['--ring' as any]: '#3ba99c',
      }}
    >
      <style>{`
        @keyframes fadeInUp {
          from { opacity: 0; transform: translateY(16px); }
          to { opacity: 1; transform: translateY(0); }
        }
      `}</style>

      {/* Header - 紧凑高度 */}
      <header
        className="sticky top-0 z-50 backdrop-blur-xl border-b transition-colors"
        style={{ background: 'var(--header-bg)', borderColor: 'var(--border)' }}
      >
        <div className="max-w-6xl mx-auto px-6 h-14 flex items-center justify-between relative z-20">
          <Link to="/" className="flex items-center gap-2.5">
            <div
              className="w-8 h-8 rounded-lg flex items-center justify-center text-white font-bold text-sm"
              style={{
                background: 'linear-gradient(135deg, var(--primary) 0%, var(--primary-soft) 100%)',
                boxShadow: '0 6px 16px -4px rgba(59,169,156,0.4)',
              }}
            >
              Y
            </div>
            <span className="font-bold text-base tracking-tight" style={{ color: 'var(--foreground)' }}>
              YunDu
            </span>
            <span className="hidden sm:inline text-xs" style={{ color: 'var(--muted-foreground)' }}>
              全球网络加速 · 高速稳定
            </span>
          </Link>
          <nav className="hidden md:flex items-center gap-7 text-sm">
            <a href="#pricing" className="text-[var(--muted-foreground)] hover:text-[var(--primary)] transition-colors">套餐</a>
            <a href="#world" className="text-[var(--muted-foreground)] hover:text-[var(--primary)] transition-colors">场景</a>
            <a href="#get-started" className="text-[var(--muted-foreground)] hover:text-[var(--primary)] transition-colors">开通</a>
          </nav>
          <div className="flex items-center gap-1.5">
            <LanguageSelector />
            <button
              onClick={toggleTheme}
              className="w-8 h-8 rounded-lg flex items-center justify-center hover:bg-[var(--muted)] transition-colors"
              title="切换主题"
            >
              {theme === 'light' ? (
                <Moon className="w-4 h-4 text-[var(--muted-foreground)]" />
              ) : (
                <Sun className="w-4 h-4 text-[var(--muted-foreground)]" />
              )}
            </button>
            <Link
              to="/login"
              className="px-3.5 py-1.5 text-sm font-medium rounded-lg transition-colors hover:bg-[var(--muted)] text-[var(--foreground)]"
            >
              登录
            </Link>
            <Link
              to="/register"
              className="px-3.5 py-1.5 text-sm font-medium rounded-lg text-white transition-all shadow-md hover:shadow-lg hover:-translate-y-0.5"
              style={{ background: 'linear-gradient(135deg, var(--primary) 0%, var(--primary-soft) 100%)' }}
            >
              注册
            </Link>
          </div>
        </div>
      </header>

      {/* Hero - 参考获奖设计：左文右球，紧凑 */}
      <section className="relative min-h-[calc(100svh-56px)] flex items-center overflow-hidden">
        <RealisticGlobe />

        <div className="relative z-10 max-w-6xl mx-auto w-full px-6 py-10 md:py-6">
          <div className="max-w-2xl" style={{ animation: 'fadeInUp 0.8s ease-out' }}>
            {/* 顶部小标签 */}
            <div className="inline-flex items-center gap-2 text-[11px] tracking-[0.18em] font-medium mb-6" style={{ color: 'var(--primary)' }}>
              <span className="inline-block w-1.5 h-1.5 rounded-full" style={{ background: 'var(--primary)' }} />
              <span>全球加速网络</span>
              <span className="opacity-40">·</span>
              <span>智能路由</span>
              <span className="opacity-40">·</span>
              <span>稳定连接</span>
            </div>

            {/* 超大粗体标题 */}
            <h1
              className="font-bold mb-5 leading-[1.05] tracking-tight"
              style={{ color: 'var(--foreground)', fontSize: 'clamp(42px, 6vw, 76px)' }}
            >
              连接世界
              <br />
              <span style={{ color: 'var(--primary)' }}>极速网络体验</span>
            </h1>

            {/* 副标题 */}
            <p
              className="text-base md:text-lg font-medium mb-4"
              style={{ color: 'var(--primary)' }}
            >
              覆盖全球的高速网络，为跨境办公、流媒体、海外业务提供稳定加速
            </p>

            {/* 描述 */}
            <p
              className="text-sm md:text-[15px] leading-[1.8] mb-8 max-w-xl"
              style={{ color: 'var(--muted-foreground)' }}
            >
              部署香港、东京、新加坡、首尔、迪拜、法兰克福、伦敦、纽约、洛杉矶、悉尼等国际核心节点，
              智能 BGP 路由毫秒级切换，端到端加密传输，为您带来低时延、无丢包的网络体验。
            </p>

            {/* 按钮组 */}
            <div className="flex items-center gap-3 flex-wrap mb-10">
              <a
                href="#pricing"
                className="group inline-flex items-center gap-2 px-7 py-3.5 rounded-xl text-white font-medium text-sm transition-all hover:-translate-y-0.5"
                style={{
                  background: 'linear-gradient(135deg, var(--primary) 0%, var(--primary-soft) 100%)',
                  boxShadow: '0 10px 30px -8px rgba(59,169,156,0.45)',
                }}
              >
                立即体验
                <ArrowRight className="w-4 h-4 transition-transform group-hover:translate-x-0.5" />
              </a>
              <Link
                to="/register"
                className="inline-flex items-center gap-2 px-7 py-3.5 rounded-xl font-medium text-sm border transition-all hover:bg-[var(--muted)] bg-[var(--card)]"
                style={{ borderColor: 'var(--border)', color: 'var(--foreground)' }}
              >
                免费注册
              </Link>
            </div>

            {/* 底部三小特性 */}
            <div className="flex items-center gap-8 md:gap-10 flex-wrap pt-6 border-t" style={{ borderColor: 'var(--border)' }}>
              {[
                { t: '全球节点', s: '覆盖五大洲核心入口' },
                { t: '极速体验', s: '低时延 · 无丢包' },
                { t: '安全加密', s: '端到端 TLS 保护' },
              ].map((x, i) => (
                <div key={i}>
                  <div className="text-sm font-semibold" style={{ color: 'var(--foreground)' }}>{x.t}</div>
                  <div className="text-xs mt-0.5" style={{ color: 'var(--muted-foreground)' }}>{x.s}</div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* 向下提示 */}
        <div className="absolute bottom-5 left-1/2 -translate-x-1/2 text-xs flex flex-col items-center gap-1 z-10" style={{ color: 'var(--muted-foreground)' }}>
          <span>向下了解套餐与场景</span>
          <div className="w-px h-6" style={{ background: 'linear-gradient(to bottom, rgba(59,169,156,0.5), transparent)' }} />
        </div>
      </section>

      {/* 套餐 - 紧接 Hero */}
      <section id="pricing" className="py-16 px-6 relative z-10">
        <div className="max-w-6xl mx-auto">
          <div className="flex items-end justify-between mb-10 flex-wrap gap-4">
            <div>
              <div className="text-[11px] tracking-[0.2em] font-medium mb-2" style={{ color: 'var(--primary)' }}>
                PRICING
              </div>
              <h2 className="text-2xl md:text-3xl font-bold tracking-tight" style={{ color: 'var(--foreground)' }}>
                灵活的套餐方案
              </h2>
              <p className="text-sm mt-2" style={{ color: 'var(--secondary-foreground)' }}>
                从体验到专业，总有一款适合您
              </p>
            </div>
            <Link
              to="/register"
              className="inline-flex items-center gap-1.5 text-sm font-medium transition-colors"
              style={{ color: 'var(--primary)' }}
            >
              查看全部套餐 <ArrowRight className="w-4 h-4" />
            </Link>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-5 max-w-5xl items-start">
            {displayPlans.map(plan => (
              <PlanCard key={plan.id} plan={plan} />
            ))}
          </div>
        </div>
      </section>

      <div className="max-w-6xl mx-auto px-6">
        <div className="h-px bg-gradient-to-r from-transparent via-[var(--border)] to-transparent" />
      </div>

      {/* 四张城市卡 - 自由网络的便利 */}
      <section id="world" className="py-16 px-6 relative z-10">
        <div className="max-w-6xl mx-auto">
          <div className="mb-10">
            <div className="text-[11px] tracking-[0.2em] font-medium mb-2" style={{ color: 'var(--primary)' }}>
              LIFE · WORLD · FREE
            </div>
            <h2 className="text-2xl md:text-3xl font-bold tracking-tight" style={{ color: 'var(--foreground)' }}>
              世界在你手边，自由一路同行
            </h2>
            <p className="text-sm mt-2 max-w-2xl" style={{ color: 'var(--secondary-foreground)' }}>
              从商务谈判到海岛假期，从维多利亚港的霓虹到云端直升机的视野，自由网络让每一次连接都顺畅无碍。
            </p>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {cityCards.map((c, i) => (
              <div
                key={i}
                className="group relative overflow-hidden rounded-2xl border transition-all duration-500 hover:-translate-y-1"
                style={{
                  borderColor: 'var(--border)',
                  height: '320px',
                  boxShadow: '0 10px 30px -15px rgba(59,169,156,0.25)',
                }}
              >
                <img
                  src={c.img}
                  alt={c.city}
                  className="absolute inset-0 w-full h-full object-cover transition-transform duration-700 group-hover:scale-110"
                  loading="lazy"
                />
                {/* 暖色渐变遮罩 */}
                <div
                  className="absolute inset-0"
                  style={{
                    background: 'linear-gradient(180deg, rgba(0,0,0,0) 35%, rgba(15,42,38,0.7) 100%)',
                  }}
                />
                {/* 角标城市 */}
                <div className="absolute top-4 left-4 text-white/90 text-[11px] tracking-[0.2em] font-medium flex items-center gap-1.5">
                  <span className="inline-block w-1.5 h-1.5 rounded-full" style={{ background: 'var(--primary)' }} />
                  {c.city}
                </div>
                <div className="absolute bottom-0 left-0 right-0 p-5 text-white">
                  <div className="text-lg font-semibold mb-1 flex items-center gap-2">
                    {i === 0 && <Briefcase className="w-4 h-4" style={{ color: 'var(--primary)' }} />}
                    {i === 1 && <Plane className="w-4 h-4" style={{ color: 'var(--primary)' }} />}
                    {i === 2 && <Film className="w-4 h-4" style={{ color: 'var(--primary)' }} />}
                    {i === 3 && <Wifi className="w-4 h-4" style={{ color: 'var(--primary)' }} />}
                    {c.title}
                  </div>
                  <p className="text-[13px] leading-relaxed text-white/80">{c.caption}</p>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      <div className="max-w-6xl mx-auto px-6">
        <div className="h-px bg-gradient-to-r from-transparent via-[var(--border)] to-transparent" />
      </div>

      {/* 核心优势（紧凑 6 格） */}
      <section className="py-14 px-6 relative z-10">
        <div className="max-w-6xl mx-auto">
          <div className="mb-8">
            <div className="text-[11px] tracking-[0.2em] font-medium mb-2" style={{ color: 'var(--primary)' }}>
              FEATURES
            </div>
            <h2 className="text-2xl md:text-3xl font-bold tracking-tight" style={{ color: 'var(--foreground)' }}>
              为什么选择 YunDu
            </h2>
          </div>
          <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
            {features.map((f, i) => (
              <div
                key={i}
                className="p-5 rounded-xl bg-white/70 border border-[var(--border)] backdrop-blur-sm transition-all hover:bg-[var(--card)] hover:shadow-sm"
              >
                <div className="flex items-center gap-2.5 mb-2">
                  <div
                    className="w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0"
                    style={{ background: 'rgba(59,169,156,0.10)', color: 'var(--primary)' }}
                  >
                    <f.icon className="w-4 h-4" strokeWidth={1.8} />
                  </div>
                  <h3 className="font-semibold text-sm" style={{ color: 'var(--foreground)' }}>{f.title}</h3>
                </div>
                <p className="text-[13px] leading-relaxed" style={{ color: 'var(--secondary-foreground)' }}>{f.desc}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <div className="max-w-6xl mx-auto px-6">
        <div className="h-px bg-gradient-to-r from-transparent via-[var(--border)] to-transparent" />
      </div>

      {/* 四步接入全球网络 - 放到最下面 */}
      <section id="get-started" className="py-16 px-6 relative z-10">
        <div className="max-w-6xl mx-auto">
          <div
            className="relative overflow-hidden rounded-3xl p-8 md:p-12"
            style={{
              background: 'linear-gradient(135deg, #ffffff 0%, #f2fbf9 50%, #e6f5f2 100%)',
              boxShadow: '0 20px 60px -20px rgba(59,169,156,0.18), 0 0 0 1px rgba(59,169,156,0.10)',
            }}
          >
            <div className="absolute -right-28 -top-28 w-80 h-80 rounded-full pointer-events-none"
              style={{ background: 'radial-gradient(circle, rgba(59,169,156,0.08) 0%, transparent 65%)', filter: 'blur(20px)' }}
            />
            <div className="absolute -left-16 -bottom-16 w-72 h-72 rounded-full pointer-events-none"
              style={{ background: 'radial-gradient(circle, rgba(59,169,156,0.05) 0%, transparent 60%)' }}
            />
            <div className="absolute inset-0 opacity-[0.3] pointer-events-none"
              style={{
                backgroundImage: 'linear-gradient(rgba(59,169,156,0.06) 1px, transparent 1px), linear-gradient(90deg, rgba(59,169,156,0.06) 1px, transparent 1px)',
                backgroundSize: '36px 36px',
              }}
            />

            <div className="relative">
              <div className="text-center mb-10">
                <div className="inline-flex items-center gap-2 px-4 py-1.5 rounded-full text-xs font-medium mb-4"
                  style={{ background: 'rgba(59,169,156,0.08)', color: 'var(--primary)', border: '1px solid rgba(59,169,156,0.15)' }}>
                  <Globe className="w-3.5 h-3.5" />
                  快速接入
                </div>
                <h2 className="text-2xl md:text-3xl font-bold mb-3 tracking-tight" style={{ color: 'var(--foreground)' }}>
                  四步接入全球网络
                </h2>
                <p className="text-sm max-w-lg mx-auto" style={{ color: 'var(--muted-foreground)' }}>
                  简单四步，即刻开启极速网络之旅，连接世界的每一个角落
                </p>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-4 gap-5 mb-10 relative">
                <div className="hidden md:block absolute top-10 left-[12.5%] right-[12.5%] h-px">
                  <div
                    className="w-full h-full"
                    style={{
                      background: 'linear-gradient(to right, transparent 0%, rgba(59,169,156,0.3) 20%, rgba(59,169,156,0.3) 80%, transparent 100%)',
                    }}
                  />
                </div>

                {steps.map((s, i) => (
                  <div key={i} className="relative flex flex-col items-center text-center group">
                    <div
                      className="relative w-16 h-16 rounded-full flex items-center justify-center mb-4 transition-transform group-hover:scale-105 bg-white"
                      style={{ boxShadow: '0 8px 24px -8px rgba(59,169,156,0.3), 0 0 0 1px rgba(59,169,156,0.12)' }}
                    >
                      <s.icon className="w-6 h-6" style={{ color: 'var(--primary)' }} strokeWidth={1.6} />
                      <span
                        className="absolute -top-1 -right-1 w-6 h-6 rounded-full flex items-center justify-center text-[11px] font-bold text-white"
                        style={{ background: 'linear-gradient(135deg, var(--primary) 0%, var(--primary-soft) 100%)', boxShadow: '0 4px 10px -2px rgba(59,169,156,0.5)' }}
                      >
                        {s.num}
                      </span>
                    </div>
                    <h3 className="font-semibold text-sm mb-1" style={{ color: 'var(--foreground)' }}>{s.title}</h3>
                    <p className="text-xs leading-relaxed max-w-[160px]" style={{ color: 'var(--muted-foreground)' }}>{s.desc}</p>
                  </div>
                ))}
              </div>

              <div className="text-center pt-6" style={{ borderTop: '1px solid rgba(59,169,156,0.10)' }}>
                <h3 className="text-xl md:text-2xl font-bold mb-2" style={{ color: 'var(--foreground)' }}>
                  准备好开始了吗？
                </h3>
                <p className="text-sm mb-6 max-w-md mx-auto" style={{ color: 'var(--muted-foreground)' }}>
                  注册即送体验流量，四步轻松接入，开启您的全球网络之旅
                </p>
                <Link
                  to="/register"
                  className="group inline-flex items-center gap-2 px-8 py-3.5 rounded-xl text-white font-semibold text-sm transition-all hover:-translate-y-0.5"
                  style={{
                    background: 'linear-gradient(135deg, var(--primary) 0%, var(--primary-soft) 100%)',
                    boxShadow: '0 10px 30px -8px rgba(59,169,156,0.5)',
                  }}
                >
                  免费注册账号
                  <ArrowRight className="w-4 h-4 transition-transform group-hover:translate-x-1" />
                </Link>
              </div>
            </div>
          </div>
        </div>
      </section>

      <div className="max-w-6xl mx-auto px-6">
        <div className="h-px bg-gradient-to-r from-transparent via-[var(--border)] to-transparent" />
      </div>

      {/* FAQ */}
      <section id="faq" className="py-14 px-6 relative z-10">
        <div className="max-w-3xl mx-auto">
          <div className="text-center mb-10">
            <h2 className="text-2xl md:text-3xl font-bold mb-3 tracking-tight" style={{ color: 'var(--foreground)' }}>
              常见问题
            </h2>
            <p className="text-sm" style={{ color: 'var(--muted-foreground)' }}>
              还有疑问？随时联系我们的客服团队
            </p>
          </div>

          <div className="space-y-2.5">
            {faqs.map((item, i) => (
              <div
                key={i}
                className="p-5 rounded-xl bg-white/70 border border-[var(--border)] backdrop-blur-sm transition-all hover:bg-[var(--card)] hover:border-[rgb(217_119_87_/_0.2)]"
              >
                <h3 className="font-medium text-sm mb-1.5" style={{ color: 'var(--foreground)' }}>{item.q}</h3>
                <p className="text-sm leading-relaxed" style={{ color: 'var(--secondary-foreground)' }}>{item.a}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <footer className="py-10 px-6 border-t" style={{ borderColor: 'var(--border)' }}>
        <div className="max-w-6xl mx-auto flex flex-col md:flex-row items-center justify-between gap-4 text-sm">
          <div className="flex items-center gap-2.5">
            <div
              className="w-6 h-6 rounded-md flex items-center justify-center text-white font-bold text-xs"
              style={{ background: 'linear-gradient(135deg, var(--primary) 0%, var(--primary-soft) 100%)' }}
            >
              Y
            </div>
            <span className="font-medium" style={{ color: 'var(--foreground)' }}>YunDu</span>
            <span style={{ color: 'var(--muted-foreground)' }}>© 2026 All rights reserved.</span>
          </div>
          <div className="flex items-center gap-5" style={{ color: 'var(--muted-foreground)' }}>
            <a href="#" className="hover:text-[var(--primary)] transition-colors">服务条款</a>
            <a href="#" className="hover:text-[var(--primary)] transition-colors">隐私政策</a>
            <Link to="/login" className="hover:text-[var(--primary)] transition-colors">用户登录</Link>
          </div>
        </div>
      </footer>
    </div>
  );
}

