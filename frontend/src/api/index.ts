import { request } from './client'
import type { Booking, EventSummary, Hold, SeatMap, Ticket } from './types'

export * from './types'
export { ApiError, isApiError } from './client'

/** A hold plus the moment it expires on this device's clock. */
export interface LocalHold extends Hold {
  /** Epoch ms on the local clock, corrected for server/client clock skew. */
  deadline: number
  /** Total length of the hold in ms (for the countdown progress bar). */
  ttlMs: number
}

const enc = encodeURIComponent

export const api = {
  async listEvents(): Promise<EventSummary[]> {
    const { data } = await request<{ events: EventSummary[] }>('GET', '/events')
    return data.events
  },

  async getEvent(id: string): Promise<EventSummary> {
    return (await request<EventSummary>('GET', `/events/${enc(id)}`)).data
  },

  async getSeats(id: string): Promise<SeatMap> {
    return (await request<SeatMap>('GET', `/events/${enc(id)}/seats`)).data
  },

  async createHold(input: { eventId: string; seatId: string; email: string }): Promise<LocalHold> {
    const { data, response } = await request<Hold>('POST', '/holds', input)
    // expiresAt is server time. The response Date header translates it to the
    // local clock, so the countdown is right even if this device's clock is off.
    const serverNow = Date.parse(response.headers.get('Date') ?? '')
    const skew = Number.isFinite(serverNow) ? serverNow - Date.now() : 0
    const deadline = Date.parse(data.expiresAt) - skew
    return { ...data, deadline, ttlMs: deadline - Date.now() }
  },

  async releaseHold(holdId: string): Promise<void> {
    await request<void>('DELETE', `/holds/${enc(holdId)}`)
  },

  async checkout(holdId: string, input: { cardNumber: string; cardName: string }): Promise<Booking> {
    return (await request<Booking>('POST', `/holds/${enc(holdId)}/checkout`, input)).data
  },

  async getBooking(id: string): Promise<Booking> {
    return (await request<Booking>('GET', `/bookings/${enc(id)}`)).data
  },

  async listTickets(email: string): Promise<Ticket[]> {
    const { data } = await request<{ tickets: Ticket[] }>('GET', `/tickets?email=${enc(email)}`)
    return data.tickets
  },
}
