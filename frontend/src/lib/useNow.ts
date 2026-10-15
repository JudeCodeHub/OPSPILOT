import { useEffect, useState } from 'react'

/** Current time (epoch ms), re-rendering every `intervalMs` while `active`. */
export function useNow(intervalMs = 250, active = true): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active) return
    const id = window.setInterval(() => setNow(Date.now()), intervalMs)
    return () => window.clearInterval(id)
  }, [intervalMs, active])
  return now
}
