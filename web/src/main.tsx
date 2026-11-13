import { lazy, StrictMode, Suspense } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import './index.css'
import Layout from './components/Layout'
import { Spinner } from './components/ui'
import Login from './pages/Login'
import ChangePassword from './pages/ChangePassword'
import Dashboard from './pages/Dashboard'
import Channels from './pages/Channels'
import Tokens from './pages/Tokens'
import Logs from './pages/Logs'
import Settings from './pages/Settings'
import Playground from './pages/Playground'

// Usage 页面依赖图表库，按路由懒加载，避免给首屏 bundle 增加体积。
const Usage = lazy(() => import('./pages/Usage'))

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Suspense fallback={<div className="flex justify-center py-20"><Spinner className="h-6 w-6" /></div>}>
          <Routes>
            <Route path="/login" element={<Login />} />
            <Route element={<Layout />}>
              <Route path="/" element={<Dashboard />} />
              <Route path="/change-password" element={<ChangePassword />} />
              <Route path="/channels" element={<Channels />} />
              <Route path="/tokens" element={<Tokens />} />
              <Route path="/logs" element={<Logs />} />
              <Route path="/playground" element={<Playground />} />
              <Route path="/usage" element={<Usage />} />
              <Route path="/settings" element={<Settings />} />
            </Route>
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Suspense>
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
