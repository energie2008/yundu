import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useEffect } from 'react';
import { ThemeProvider } from './lib/theme';
import { useAuthStore } from './lib/auth';
import { ProtectedRoute } from './components/ProtectedRoute';
import { Landing } from './pages/Landing';
import { Login } from './pages/Login';
import { Register } from './pages/Register';
import { Dashboard } from './pages/Dashboard';
import { Plans } from './pages/Plans';
import { Checkout } from './pages/Checkout';
import Orders from './pages/Orders';
import Tickets from './pages/Tickets';
import Profile from './pages/Profile';
import Invite from './pages/Invite';
import Docs from './pages/Docs';
import Notifications from './pages/Notifications';
import Announcements from './pages/Announcements';
import ForgotPassword from './pages/ForgotPassword';
import ResetPassword from './pages/ResetPassword';
import VerifyEmail from './pages/VerifyEmail';
import OrderDetail from './pages/OrderDetail';
import { Layout } from './components/Layout';
import { ToastProvider } from './lib/toast';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false, staleTime: 30000 },
  },
});

function AuthInit() {
  const init = useAuthStore(s => s.init);
  const isAuthenticated = useAuthStore(s => s.isAuthenticated);
  const fetchMe = useAuthStore(s => s.fetchMe);

  useEffect(() => {
    init();
  }, [init]);

  useEffect(() => {
    if (isAuthenticated) {
      fetchMe();
    }
  }, [isAuthenticated, fetchMe]);

  return null;
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <ToastProvider>
          <BrowserRouter>
            <AuthInit />
            <Routes>
              <Route path="/" element={<Landing />} />
              <Route path="/login" element={<Login />} />
              <Route path="/register" element={<Register />} />
              <Route path="/forgot-password" element={<ForgotPassword />} />
              <Route path="/reset-password" element={<ResetPassword />} />
              <Route
                path="/dashboard"
                element={
                  <ProtectedRoute>
                    <Layout />
                  </ProtectedRoute>
                }
              >
                <Route index element={<Dashboard />} />
                <Route path="plans" element={<Plans />} />
                <Route path="checkout" element={<Checkout />} />
                <Route path="orders" element={<Orders />} />
                <Route path="orders/:id" element={<OrderDetail />} />
                <Route path="tickets" element={<Tickets />} />
                <Route path="profile" element={<Profile />} />
                <Route path="invite" element={<Invite />} />
                <Route path="knowledge" element={<Docs />} />
                <Route path="notifications" element={<Notifications />} />
                <Route path="announcements" element={<Announcements />} />
                <Route path="verify-email" element={<VerifyEmail />} />
              </Route>
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </BrowserRouter>
        </ToastProvider>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
