import { useState, type ReactNode } from 'react'
import { motion } from 'motion/react'
import { toFriendlyError } from '../api/errors'

/** Small, copyable "Reference: {request_id}" used on every error. */
export function Reference({ id }: { id?: string }) {
  const [copied, setCopied] = useState(false)
  if (!id) return null

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(id)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1600)
    } catch {
      // Clipboard blocked: the ID is still selectable text.
    }
  }

  return (
    <p className="mt-3 flex flex-wrap items-center gap-2 text-xs text-ink-muted">
      <span>Reference:</span>
      <code className="select-all rounded-md bg-white/5 px-2 py-1 font-mono text-[0.7rem] text-ink-soft">{id}</code>
      <button type="button" onClick={copy} className="rounded-md px-2 py-1 text-neon-cyan hover:bg-white/5">
        {copied ? 'Copied' : 'Copy'}
      </button>
    </p>
  )
}

interface ErrorStateProps {
  error: unknown
  onRetry?: () => void
  retrying?: boolean
  /** Extra actions, e.g. a "Pick another seat" link. */
  children?: ReactNode
  compact?: boolean
}

/** A designed error: friendly title and text, reference ID, and retry when it makes sense. */
export function ErrorState({ error, onRetry, retrying, children, compact }: ErrorStateProps) {
  const e = toFriendlyError(error)
  return (
    <motion.div
      role="alert"
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      className={`rounded-2xl border border-danger/30 bg-danger/[0.07] ${compact ? 'p-4' : 'p-6 sm:p-8'}`}
    >
      <div className="flex items-start gap-3">
        <span aria-hidden="true" className="mt-0.5 grid size-8 shrink-0 place-items-center rounded-full bg-danger/15 text-danger">
          <svg viewBox="0 0 20 20" className="size-4" fill="currentColor">
            <path d="M10 2a8 8 0 1 0 0 16 8 8 0 0 0 0-16Zm0 4a1 1 0 0 1 1 1v3.5a1 1 0 1 1-2 0V7a1 1 0 0 1 1-1Zm0 8.5a1.1 1.1 0 1 1 0-2.2 1.1 1.1 0 0 1 0 2.2Z" />
          </svg>
        </span>
        <div className="min-w-0 flex-1">
          <h3 className={`font-display font-semibold text-ink ${compact ? 'text-base' : 'text-lg'}`}>{e.title}</h3>
          <p className="mt-1 text-sm text-ink-soft">{e.description}</p>
          {(onRetry && e.retryable) || children ? (
            <div className="mt-4 flex flex-wrap gap-3">
              {onRetry && e.retryable && (
                <button type="button" className="btn-ghost" onClick={onRetry} disabled={retrying}>
                  {retrying ? 'Retrying…' : 'Try again'}
                </button>
              )}
              {children}
            </div>
          ) : null}
          <Reference id={e.requestId} />
        </div>
      </div>
    </motion.div>
  )
}
