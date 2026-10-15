import { Component, type ErrorInfo, type ReactNode } from 'react'
import { isRouteErrorResponse, Link, useRouteError } from 'react-router'

function Fallback({ title, description }: { title: string; description: string }) {
  return (
    <div className="grid min-h-dvh place-items-center px-4">
      <div className="glass max-w-lg rounded-3xl p-8 text-center sm:p-10">
        <div aria-hidden="true" className="mx-auto mb-6 size-16 rounded-2xl bg-gradient-to-br from-neon-cyan via-neon-violet to-neon-pink opacity-80 blur-[1px]" />
        <h1 className="text-3xl font-bold">{title}</h1>
        <p className="mt-3 text-ink-soft">{description}</p>
        <div className="mt-8 flex justify-center gap-3">
          <a href="/" className="btn-primary">Back to events</a>
          <button type="button" className="btn-ghost" onClick={() => window.location.reload()}>
            Reload
          </button>
        </div>
      </div>
    </div>
  )
}

/** Last-resort boundary around the whole app for unexpected render errors. */
export class AppErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Unhandled UI error', error, info.componentStack)
  }

  render() {
    if (this.state.failed) {
      return <Fallback title="This page hit a wrong note" description="Something unexpected happened while showing this page. Reloading usually fixes it." />
    }
    return this.props.children
  }
}

/** Error element for the router (render errors and unknown routes). */
export function RouteErrorPage() {
  const error = useRouteError()
  if (isRouteErrorResponse(error) && error.status === 404) {
    return <NotFound />
  }
  console.error('Route error', error)
  return <Fallback title="This page hit a wrong note" description="Something unexpected happened while showing this page. Reloading usually fixes it." />
}

export function NotFound() {
  return (
    <div className="mx-auto max-w-xl px-4 py-24 text-center">
      <p className="text-gradient font-display text-7xl font-bold">404</p>
      <h1 className="mt-4 text-2xl font-bold">This stage is empty</h1>
      <p className="mt-2 text-ink-soft">The page you’re looking for doesn’t exist.</p>
      <Link to="/" className="btn-primary mt-8">
        Browse events
      </Link>
    </div>
  )
}
