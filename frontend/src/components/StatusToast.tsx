import { useState } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { useApiDegraded } from '../api/health'

/** Global toast shown when several API calls in a row have failed. */
export function StatusToast() {
  const degraded = useApiDegraded()
  // Dismissal lasts until the next time the API becomes degraded.
  const [dismissedFor, setDismissedFor] = useState<boolean | null>(null)
  const [lastDegraded, setLastDegraded] = useState(degraded)
  if (degraded !== lastDegraded) {
    setLastDegraded(degraded)
    if (!degraded) setDismissedFor(null)
  }
  const show = degraded && dismissedFor === null

  return (
    <div aria-live="polite" className="pointer-events-none fixed inset-x-0 bottom-4 z-40 flex justify-center px-4">
      <AnimatePresence>
        {show && (
          <motion.div
            role="status"
            initial={{ opacity: 0, y: 24, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 24 }}
            className="glass pointer-events-auto flex max-w-md items-start gap-3 rounded-2xl px-4 py-3"
          >
            <span aria-hidden="true" className="mt-1.5 size-2.5 shrink-0 animate-pulse rounded-full bg-neon-amber shadow-[0_0_12px_2px_rgb(251_191_36/0.6)]" />
            <div className="text-sm">
              <p className="font-semibold text-ink">Some services are degraded</p>
              <p className="text-ink-soft">Things may be slow or fail for a moment. We’re on it.</p>
            </div>
            <button
              type="button"
              onClick={() => setDismissedFor(true)}
              className="ml-2 rounded-md px-2 py-1 text-xs text-ink-muted hover:bg-white/10 hover:text-ink"
              aria-label="Dismiss"
            >
              ✕
            </button>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}
