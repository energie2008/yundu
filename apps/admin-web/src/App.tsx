import { useEffect } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter } from 'react-router-dom'
import { ToastProvider } from '@airport/ui'
import { useAuthStore } from '@/lib/auth'
import { ProtectedRoute } from '@/components/ProtectedRoute'
import { Layout } from '@/components/layout/Layout'
import Login from '@/pages/Login'
import Dashboard from '@/pages/Dashboard'
import Nodes from '@/pages/Nodes'
import NodeGroups from '@/pages/NodeGroups'
import Machines from '@/pages/Machines'
import Plans from '@/pages/Plans'
import Users from '@/pages/Users'
import Orders from '@/pages/Orders'
import Settings from '@/pages/Settings'
import Profile from '@/pages/Profile'
import Tickets from '@/pages/Tickets'
import Announcements from '@/pages/Announcements'
import Coupons from '@/pages/Coupons'
import GiftCards from '@/pages/GiftCards'
import Commissions from '@/pages/Commissions'
import Payments from '@/pages/Payments'
import Knowledge from '@/pages/Knowledge'
import SystemTheme from '@/pages/SystemTheme'
import SystemPlugin from '@/pages/SystemPlugin'
import AuditLogs from '@/pages/AuditLogs'
import SubscriptionPreview from '@/pages/SubscriptionPreview'
import DiagnosticsChannels from '@/pages/DiagnosticsChannels'
import DiagnosticsAI from '@/pages/DiagnosticsAI'
import ExperienceDashboard from '@/pages/ExperienceDashboard'
import NodeDoctor from '@/pages/NodeDoctor'
import ExposureManager from '@/pages/ExposureManager'
import ProtocolRegistry from '@/pages/ProtocolRegistry'
import TLSCertificates from '@/pages/TLSCertificates'
import ConfigImporter from '@/pages/ConfigImporter'
import ClientCompat from '@/pages/ClientCompat'
import Servers from '@/pages/Servers'
import RouteRuleSets from '@/pages/RouteRuleSets'
import ProxyChains from '@/pages/ProxyChains'
import RoutePolicies from '@/pages/RoutePolicies'
import FinanceAudit from '@/pages/FinanceAudit'
import Notifications from '@/pages/Notifications'
import MailTemplates from '@/pages/MailTemplates'
import SubscriptionTemplates from '@/pages/SubscriptionTemplates'
import Deployments from '@/pages/Deployments'
import Presets from '@/pages/Presets'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 10000,
    },
  },
})

function AppRoutes() {
  const init = useAuthStore((state) => state.init)

  useEffect(() => {
    init()
  }, [init])

  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/"
        element={
          <ProtectedRoute>
            <Layout />
          </ProtectedRoute>
        }
      >
        <Route index element={<Navigate to="/dashboard" replace />} />
        <Route path="dashboard" element={<Dashboard />} />

        <Route path="nodes" element={<Nodes />} />
        <Route path="node-groups" element={<NodeGroups />} />
        <Route path="presets" element={<Presets />} />
        <Route path="machines" element={<Machines />} />
        <Route path="servers" element={<Servers />} />
        <Route path="rule-sets" element={<RouteRuleSets />} />
        <Route path="proxy-chains" element={<ProxyChains />} />
        <Route path="route-policies" element={<RoutePolicies />} />
        <Route path="deployments" element={<Deployments />} />

        <Route path="plans" element={<Plans />} />
        <Route path="subscription-preview" element={<SubscriptionPreview />} />
        <Route path="subscribe-templates" element={<SubscriptionTemplates />} />
        <Route path="mail-templates" element={<MailTemplates />} />

        <Route path="users" element={<Users />} />
        <Route path="orders" element={<Orders />} />

        <Route path="payments" element={<Payments />} />
        <Route path="coupons" element={<Coupons />} />
        <Route path="gift-cards" element={<GiftCards />} />
        <Route path="commissions" element={<Commissions />} />

        <Route path="tickets" element={<Tickets />} />
        <Route path="announcements" element={<Announcements />} />
        <Route path="knowledge" element={<Knowledge />} />

        <Route path="system/config" element={<Settings />} />
        <Route path="system/theme" element={<SystemTheme />} />
        <Route path="system/plugin" element={<SystemPlugin />} />
        <Route path="system/audit" element={<AuditLogs />} />

        <Route path="profile" element={<Profile />} />

        <Route path="diagnostics/channels" element={<DiagnosticsChannels />} />
        <Route path="diagnostics/ai" element={<DiagnosticsAI />} />
        <Route path="experience" element={<ExperienceDashboard />} />
        <Route path="doctor" element={<NodeDoctor />} />
        <Route path="exposure" element={<ExposureManager />} />
        <Route path="protocols" element={<ProtocolRegistry />} />
        <Route path="certificates" element={<TLSCertificates />} />
        <Route path="importer" element={<ConfigImporter />} />
        <Route path="compat" element={<ClientCompat />} />
        <Route path="finance/audit" element={<FinanceAudit />} />
        <Route path="notifications" element={<Notifications />} />
      </Route>
      <Route path="*" element={<Navigate to="/dashboard" replace />} />
    </Routes>
  )
}

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <ToastProvider>
          <AppRoutes />
        </ToastProvider>
      </BrowserRouter>
    </QueryClientProvider>
  )
}

export default App
