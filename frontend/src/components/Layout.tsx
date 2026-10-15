import { AnimatePresence, motion } from 'motion/react'
import { Link, NavLink, ScrollRestoration, useLocation, useOutlet } from 'react-router'
import { StatusToast } from './StatusToast'

function Logo() {
  return (
    <Link to="/" className="flex items-center gap-2.5" aria-label="OpsPilot Live, home">
      <svg viewBox="0 0 64 64" className="size-8" aria-hidden="true">
        <defs>
          <linearGradient id="logo-g" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0" stopColor="#22d3ee" />
            <stop offset=".5" stopColor="#a855f7" />
            <stop offset="1" stopColor="#f472b6" />
          </linearGradient>
        </defs>
        <rect width="64" height="64" rx="16" fill="#141127" />
        <path d="M14 40c6-14 10-14 14 0s8 14 12 0 8-14 10-8" fill="none" stroke="url(#logo-g)" strokeWidth="5" strokeLinecap="round" />
      </svg>
      <span className="font-display text-lg font-bold tracking-tight">
        OpsPilot <span className="text-gradient">Live</span>
      </span>
    </Link>
  )
}

const navClass = ({ isActive }: { isActive: boolean }) =>
  `rounded-lg px-3 py-2 text-sm font-medium transition ${isActive ? 'bg-white/10 text-ink' : 'text-ink-soft hover:bg-white/5 hover:text-ink'}`

export function Layout() {
  const location = useLocation()
  const outlet = useOutlet()

  return (
    <div className="flex min-h-dvh flex-col">
      <a href="#main" className="sr-only focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-50 focus:rounded-lg focus:bg-night-800 focus:px-4 focus:py-2">
        Skip to content
      </a>
      <header className="sticky top-0 z-30 border-b border-white/5 bg-night-950/70 backdrop-blur-xl">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-4 sm:px-6">
          <Logo />
          <nav aria-label="Main" className="flex items-center gap-1">
            <NavLink to="/" end className={navClass}>
              Events
            </NavLink>
            <NavLink to="/tickets" className={navClass}>
              My tickets
            </NavLink>
          </nav>
        </div>
      </header>

      <AnimatePresence mode="wait" initial={false}>
        <motion.main
          id="main"
          key={location.pathname}
          className="flex-1"
          initial={{ opacity: 0, y: 12 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -8 }}
          transition={{ duration: 0.25, ease: 'easeOut' }}
        >
          {outlet}
        </motion.main>
      </AnimatePresence>

      <footer className="mt-16 border-t border-white/5">
        <div className="mx-auto flex max-w-6xl flex-col gap-2 px-4 py-8 text-xs text-ink-muted sm:flex-row sm:justify-between sm:px-6">
          <p>OpsPilot Live — a demo ticketing app. No real payments are taken.</p>
          <p>All artists and venues are fictional.</p>
        </div>
      </footer>

      <StatusToast />
      <ScrollRestoration />
    </div>
  )
}
