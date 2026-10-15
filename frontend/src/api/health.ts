import { useSyncExternalStore } from 'react'

/**
 * Tracks consecutive server-side API failures (5xx and network errors).
 * After DEGRADED_AFTER failures in a row the app shows a global
 * "Some services are degraded" toast; any success clears it.
 */
const DEGRADED_AFTER = 3

let consecutiveFailures = 0
const listeners = new Set<() => void>()

function emit() {
  for (const listener of listeners) listener()
}

export const apiHealth = {
  failure() {
    consecutiveFailures += 1
    if (consecutiveFailures === DEGRADED_AFTER) emit()
  },
  success() {
    if (consecutiveFailures === 0) return
    const wasDegraded = consecutiveFailures >= DEGRADED_AFTER
    consecutiveFailures = 0
    if (wasDegraded) emit()
  },
  isDegraded() {
    return consecutiveFailures >= DEGRADED_AFTER
  },
  subscribe(listener: () => void) {
    listeners.add(listener)
    return () => {
      listeners.delete(listener)
    }
  },
}

/** True while several API calls in a row have failed. */
export function useApiDegraded(): boolean {
  return useSyncExternalStore(apiHealth.subscribe, apiHealth.isDegraded, () => false)
}
