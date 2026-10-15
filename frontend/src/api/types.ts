// Types matching the JSON returned by the gateway (/api/*).

export type Theme = 'aurora' | 'neon' | 'sunset' | 'ocean' | 'ember' | 'prism'

export interface Venue {
  id: string
  name: string
  city: string
}

export interface EventSummary {
  id: string
  name: string
  artist: string
  startsAt: string
  description: string
  theme: Theme | string
  venue: Venue
  minPriceCents: number
  currency: string
  seatsAvailable: number
  seatsTotal: number
}

export type Section = 'VIP' | 'Premium' | 'Standard'
export type SeatStatus = 'available' | 'held' | 'sold'

export interface Seat {
  seatId: string
  section: Section
  row: string
  number: number
  priceCents: number
  currency: string
  status: SeatStatus
}

export interface SeatMap {
  eventId: string
  seats: Seat[]
}

export interface Hold {
  holdId: string
  eventId: string
  seatId: string
  expiresAt: string
  priceCents: number
  currency: string
}

export interface Booking {
  id: string
  eventId: string
  seatId: string
  email: string
  amountCents: number
  currency: string
  chargeId: string
  status: string
  createdAt: string
}

export interface Ticket {
  code: string
  bookingId: string
  eventId: string
  eventName: string
  artist: string
  startsAt: string
  theme: Theme | string
  venueName: string
  venueCity: string
  seatId: string
  section: Section | ''
  row: string
  number: number
  email: string
  issuedAt: string
}

export interface ApiErrorBody {
  error: { code: string; message: string; request_id?: string }
}
