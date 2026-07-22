import {
  LayoutDashboard,
  Server,
  HardDrive,
  Users,
  Route,
  Globe,
  Stethoscope,
  Upload,
  Smartphone,
  Ticket,
  Megaphone,
  Settings,
  CreditCard,
  Eye,
  Activity,
  Zap,
  Radio,
  Receipt,
  Boxes,
  Lock,
  Layers,
  Gift,
  Wallet,
  BookOpen,
  Palette,
  Plug,
  FileText,
  BarChart3,
  UserCog,
  Cpu,
  Network,
  Bell,
  Gauge,
  Mail,
  FileCode2,
  Rocket,
} from 'lucide-react'

export interface NavItem {
  label: string
  path: string
  icon?: React.ComponentType<{ className?: string }>
  badge?: string
  badgeColor?: string
}

export interface NavGroup {
  label: string
  items: NavItem[]
  collapsible?: boolean
}

export const sidebarGroups: NavGroup[] = [
  {
    label: '概览',
    items: [
      { label: '仪表盘', path: '/dashboard', icon: LayoutDashboard },
    ],
  },
  {
    label: '系统管理',
    items: [
      { label: '系统配置', path: '/system/config', icon: Settings },
      { label: '主题设置', path: '/system/theme', icon: Palette },
      { label: '插件管理', path: '/system/plugin', icon: Plug },
      { label: '邮件模板', path: '/mail-templates', icon: Mail },
      { label: '审计日志', path: '/system/audit', icon: FileText },
    ],
  },
  {
    label: '节点管理',
    items: [
      { label: '节点管理', path: '/nodes', icon: Server },
      { label: '协议预设', path: '/presets', icon: Settings },
      { label: '节点分组', path: '/node-groups', icon: Layers },
      { label: '分流规则', path: '/rule-sets', icon: Globe },
      { label: '路由管理', path: '/route-policies', icon: Route },
      { label: '中转链路', path: '/proxy-chains', icon: Zap },
      { label: '服务器管理', path: '/servers', icon: HardDrive },
      { label: '部署管理', path: '/deployments', icon: Rocket },
    ],
  },
  {
    label: '订阅与套餐',
    items: [
      { label: '套餐管理', path: '/plans', icon: CreditCard },
      { label: '订阅预览', path: '/subscription-preview', icon: Eye },
      { label: '订阅模板', path: '/subscribe-templates', icon: FileCode2 },
    ],
  },
  {
    label: '用户与订单',
    items: [
      { label: '用户管理', path: '/users', icon: Users },
      { label: '订单管理', path: '/orders', icon: Receipt },
    ],
  },
  {
    label: '财务中心',
    items: [
      { label: '支付配置', path: '/payments', icon: CreditCard },
      { label: '优惠券管理', path: '/coupons', icon: Gift },
      { label: '礼品卡管理', path: '/gift-cards', icon: Gift },
      { label: '返利提现', path: '/commissions', icon: Wallet },
    ],
  },
  {
    label: '客服中心',
    items: [
      { label: '工单中心', path: '/tickets', icon: Ticket },
      { label: '公告管理', path: '/announcements', icon: Megaphone },
      { label: '知识库', path: '/knowledge', icon: BookOpen },
    ],
  },
  {
    label: 'YunDu增强',
    collapsible: true,
    items: [
      { label: 'AI 诊断', path: '/diagnostics/ai', icon: Zap, badge: 'AI', badgeColor: 'bg-indigo-500' },
      { label: '通道健康', path: '/diagnostics/channels', icon: Radio, badge: 'NEW', badgeColor: 'bg-emerald-500' },
      { label: '节点体验', path: '/experience', icon: Gauge, badge: 'NEW', badgeColor: 'bg-violet-500' },
      { label: '节点体检', path: '/doctor', icon: Stethoscope },
      { label: '边缘暴露', path: '/exposure', icon: Globe, badge: 'BETA', badgeColor: 'bg-amber-500' },
      { label: '协议注册', path: '/protocols', icon: Boxes, badge: 'BETA', badgeColor: 'bg-amber-500' },
      { label: 'TLS 证书', path: '/certificates', icon: Lock, badge: 'BETA', badgeColor: 'bg-amber-500' },
      { label: '配置导入', path: '/importer', icon: Upload },
      { label: '客户端兼容', path: '/compat', icon: Smartphone },
    ],
  },
]

export const allNavItems: NavItem[] = [
  ...sidebarGroups.flatMap((g) => g.items),
  { label: '我的', path: '/profile', icon: UserCog },
  { label: '数据统计', path: '/stats', icon: BarChart3 },
]

export function getPageTitle(pathname: string): string {
  const item = allNavItems.find((i) => pathname.startsWith(i.path))
  return item?.label || '仪表盘'
}

export type TabGroup = 'main' | 'nodes' | 'finance' | 'support' | 'system' | 'yundu'

export const TAB_GROUPS: Record<TabGroup, NavItem[]> = {
  main: [
    { label: '仪表盘', path: '/dashboard', icon: LayoutDashboard },
    { label: '节点', path: '/nodes', icon: Server },
    { label: '用户', path: '/users', icon: Users },
    { label: '我的', path: '/profile', icon: UserCog },
  ],
  nodes: [
    { label: '节点', path: '/nodes', icon: Server },
    { label: '分组', path: '/node-groups', icon: Layers },
    { label: '分流', path: '/rule-sets', icon: Globe },
    { label: '路由', path: '/route-policies', icon: Route },
    { label: '中转', path: '/proxy-chains', icon: Zap },
    { label: '服务器', path: '/servers', icon: HardDrive },
    { label: '部署', path: '/deployments', icon: Rocket },
    { label: '主页', path: '/dashboard', icon: LayoutDashboard },
  ],
  finance: [
    { label: '订单', path: '/orders', icon: Receipt },
    { label: '套餐', path: '/plans', icon: CreditCard },
    { label: '优惠券', path: '/coupons', icon: Gift },
    { label: '支付', path: '/payments', icon: CreditCard },
    { label: '返利', path: '/commissions', icon: Wallet },
    { label: '主页', path: '/dashboard', icon: LayoutDashboard },
  ],
  support: [
    { label: '工单', path: '/tickets', icon: Ticket },
    { label: '公告', path: '/announcements', icon: Megaphone },
    { label: '知识库', path: '/knowledge', icon: BookOpen },
    { label: '主页', path: '/dashboard', icon: LayoutDashboard },
  ],
  system: [
    { label: '配置', path: '/system/config', icon: Settings },
    { label: '主题', path: '/system/theme', icon: Palette },
    { label: '插件', path: '/system/plugin', icon: Plug },
    { label: '审计', path: '/system/audit', icon: FileText },
    { label: '主页', path: '/dashboard', icon: LayoutDashboard },
  ],
  yundu: [
    { label: 'AI', path: '/diagnostics/ai', icon: Zap },
    { label: '通道', path: '/diagnostics/channels', icon: Radio },
    { label: '体验', path: '/experience', icon: Gauge },
    { label: '体检', path: '/doctor', icon: Stethoscope },
    { label: '暴露', path: '/exposure', icon: Globe },
    { label: '主页', path: '/dashboard', icon: LayoutDashboard },
  ],
}

export const TAB_GROUP_LABELS: Record<TabGroup, string> = {
  main: '主功能',
  nodes: '节点',
  finance: '财务',
  support: '客服',
  system: '系统',
  yundu: '增强',
}

export const TAB_GROUP_COLORS: Record<TabGroup, string> = {
  main: 'bg-indigo-500',
  nodes: 'bg-emerald-500',
  finance: 'bg-amber-500',
  support: 'bg-sky-500',
  system: 'bg-zinc-500',
  yundu: 'bg-violet-500',
}

export function getTabGroup(pathname: string): TabGroup {
  if (pathname.startsWith('/nodes') || pathname.startsWith('/node-groups') || pathname.startsWith('/machines') || pathname.startsWith('/servers') || pathname.startsWith('/rule-sets') || pathname.startsWith('/route-policies') || pathname.startsWith('/proxy-chains') || pathname.startsWith('/deployments')) return 'nodes'
  if (pathname.startsWith('/plans') || pathname.startsWith('/orders') || pathname.startsWith('/payments') || pathname.startsWith('/coupons') || pathname.startsWith('/gift-cards') || pathname.startsWith('/commissions') || pathname.startsWith('/finance/') || pathname.startsWith('/subscribe-templates')) return 'finance'
  if (pathname.startsWith('/tickets') || pathname.startsWith('/announcements') || pathname.startsWith('/knowledge') || pathname.startsWith('/notifications')) return 'support'
  if (pathname.startsWith('/system/') || pathname.startsWith('/mail-templates')) return 'system'
  if (pathname.startsWith('/diagnostics') || pathname.startsWith('/doctor') || pathname.startsWith('/exposure') || pathname.startsWith('/protocols') || pathname.startsWith('/certificates') || pathname.startsWith('/importer') || pathname.startsWith('/compat') || pathname.startsWith('/experience')) return 'yundu'
  return 'main'
}
