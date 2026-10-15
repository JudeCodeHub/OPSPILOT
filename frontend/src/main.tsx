import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MotionConfig } from 'motion/react'
import { createBrowserRouter, RouterProvider } from 'react-router'

import '@fontsource/inter/400.css'
import '@fontsource/inter/500.css'
import '@fontsource/inter/600.css'
import '@fontsource/space-grotesk/500.css'
import '@fontsource/space-grotesk/700.css'
import './index.css'

import { isApiError } from './api'
import { AppErrorBoundary, NotFound, RouteErrorPage } from './components/ErrorBoundary'
import { Layout } from './components/Layout'
import { Checkout } from './pages/Checkout'
import { Confirmation } from './pages/Confirmation'
import { EventDetail } from './pages/EventDetail'
import { Home } from './pages/Home'
import { MyTickets } from './pages/MyTickets'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10_000,
      refetchOnWindowFocus: false,
      // One quick retry for server errors; a 4xx will not change on retry.
      retry: (count, err) => count < 1 && !(isApiError(err) && err.status >= 400 && err.status < 500),
      retryDelay: 800,
    },
    mutations: { retry: false },
  },
})

const router = createBrowserRouter([
  {
    element: <Layout />,
    errorElement: <RouteErrorPage />,
    children: [
      { path: '/', element: <Home /> },
      { path: '/events/:id', element: <EventDetail /> },
      { path: '/checkout/:holdId', element: <Checkout /> },
      { path: '/bookings/:id', element: <Confirmation /> },
      { path: '/tickets', element: <MyTickets /> },
      { path: '*', element: <NotFound /> },
    ],
  },
])

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AppErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <MotionConfig reducedMotion="user">
          <RouterProvider router={router} />
        </MotionConfig>
      </QueryClientProvider>
    </AppErrorBoundary>
  </StrictMode>,
)
