import { isApiError } from './client'

export interface FriendlyError {
  title: string
  description: string
  /** Whether trying the same action again can reasonably succeed. */
  retryable: boolean
  code: string
  requestId?: string
}

type Copy = Pick<FriendlyError, 'title' | 'description' | 'retryable'>

const copy: Record<string, Copy> = {
  SEAT_UNAVAILABLE: {
    title: 'Someone just grabbed that seat',
    description: 'Pick another one — the map now shows what’s still free.',
    retryable: false,
  },
  SEAT_NOT_FOUND: {
    title: 'That seat doesn’t exist',
    description: 'Please choose a seat from the map.',
    retryable: false,
  },
  HOLD_EXPIRED: {
    title: 'Your hold has expired',
    description: 'Seats are held for a few minutes. Pick a seat again to continue.',
    retryable: false,
  },
  PAYMENT_DECLINED: {
    title: 'Your card was declined',
    description: 'Nothing was charged. Please try a different card.',
    retryable: false,
  },
  INVALID_CARD: {
    title: 'Check your card details',
    description: 'The card number doesn’t look right.',
    retryable: false,
  },
  PAYMENT_TIMEOUT: {
    title: 'Payment is taking longer than usual',
    description: 'You won’t be charged twice — it’s safe to try again.',
    retryable: true,
  },
  PAYMENT_UNAVAILABLE: {
    title: 'We couldn’t reach the payment provider',
    description: 'Your seat is still held. Please try again in a moment.',
    retryable: true,
  },
  UPSTREAM_UNAVAILABLE: {
    title: 'We’re having trouble reaching our servers',
    description: 'This is on our side, not yours. Please try again in a moment.',
    retryable: true,
  },
  UPSTREAM_TIMEOUT: {
    title: 'This is taking longer than expected',
    description: 'Our servers are slow right now. Please try again.',
    retryable: true,
  },
  NETWORK_ERROR: {
    title: 'You seem to be offline',
    description: 'Check your connection and try again.',
    retryable: true,
  },
  EVENT_NOT_FOUND: {
    title: 'Event not found',
    description: 'It may have been removed, or the link is wrong.',
    retryable: false,
  },
  BOOKING_NOT_FOUND: {
    title: 'Booking not found',
    description: 'Check the link, or look up your tickets with your email.',
    retryable: false,
  },
  INVALID_REQUEST: {
    title: 'Please check your details',
    description: 'Something in the form isn’t quite right.',
    retryable: false,
  },
}

const fallback: Copy = {
  title: 'Something went wrong on our side',
  description: 'Please try again. If it keeps happening, share the reference below with support.',
  retryable: true,
}

/** Maps any thrown value to a message that is safe and helpful to show. */
export function toFriendlyError(err: unknown): FriendlyError {
  if (!isApiError(err)) return { ...fallback, code: 'CLIENT_ERROR' }
  const known = copy[err.code] ?? fallback
  // Validation messages from the server are specific and safe to show as-is.
  const description = err.code === 'INVALID_REQUEST' || err.code === 'INVALID_CARD' ? err.message : known.description
  return { ...known, description, code: err.code, requestId: err.requestId }
}
