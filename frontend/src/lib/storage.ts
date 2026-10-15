// Browser storage can be unavailable (private mode, blocked cookies) and then
// throws on access. Every read and write here is wrapped so the app still works.

import type { EventSummary, LocalHold } from '../api'

function read(store: () => Storage, key: string): string | null {
  try {
    return store().getItem(key)
  } catch {
    return null
  }
}

function write(store: () => Storage, key: string, value: string | null): void {
  try {
    if (value === null) store().removeItem(key)
    else store().setItem(key, value)
  } catch {
    // storage unavailable: nothing to do
  }
}

const local = () => window.localStorage
const session = () => window.sessionStorage

const EMAIL_KEY = 'opspilot.email'

export const lastEmail = {
  get: () => read(local, EMAIL_KEY) ?? '',
  set: (email: string) => write(local, EMAIL_KEY, email),
}

/** What the checkout page needs about a hold, kept for this tab only. */
export interface StoredHold {
  hold: LocalHold
  email: string
  event: Pick<EventSummary, 'id' | 'name' | 'artist' | 'startsAt' | 'theme' | 'venue'>
  section: string
}

const holdKey = (holdId: string) => `opspilot.hold.${holdId}`

export const holdStore = {
  get(holdId: string): StoredHold | null {
    const raw = read(session, holdKey(holdId))
    if (!raw) return null
    try {
      return JSON.parse(raw) as StoredHold
    } catch {
      return null
    }
  },
  set(value: StoredHold) {
    write(session, holdKey(value.hold.holdId), JSON.stringify(value))
  },
  remove(holdId: string) {
    write(session, holdKey(holdId), null)
  },
}
