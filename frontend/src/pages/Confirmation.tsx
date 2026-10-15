import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AnimatePresence, motion } from 'motion/react'
import { Link, useLocation, useParams } from 'react-router'
import { api } from '../api'
import { Confetti } from '../components/Confetti'
import { ErrorState } from '../components/ErrorState'
import { TicketCard } from '../components/TicketCard'
import { formatPrice } from '../lib/format'
import { useNow } from '../lib/useNow'

/** How often to check whether the ticket has been issued. */
const TICKET_POLL_MS = 1500
/** After this long without a ticket, reassure the user that it is still coming. */
const SLOW_TICKET_MS = 15_000

export function Confirmation() {
  const { id = '' } = useParams()
  const location = useLocation()
  const celebrate = (location.state as { celebrate?: boolean } | null)?.celebrate === true

  const booking = useQuery({ queryKey: ['booking', id], queryFn: () => api.getBooking(id) })
  const email = booking.data?.email
  const eventId = booking.data?.eventId

  const event = useQuery({
    queryKey: ['event', eventId],
    queryFn: () => api.getEvent(eventId ?? ''),
    enabled: !!eventId,
  })

  const tickets = useQuery({
    queryKey: ['tickets', email],
    queryFn: () => api.listTickets(email ?? ''),
    enabled: !!email,
    // Keep polling until our ticket shows up.
    refetchInterval: (q) => (q.state.data?.some((t) => t.bookingId === id) ? false : TICKET_POLL_MS),
  })
  const ticket = tickets.data?.find((t) => t.bookingId === id)

  const [startedAt] = useState(() => Date.now())
  const now = useNow(1000, !ticket)
  const slow = !ticket && now - startedAt > SLOW_TICKET_MS

  if (booking.isError) {
    return (
      <div className="mx-auto max-w-2xl px-4 py-16">
        <ErrorState error={booking.error} onRetry={() => booking.refetch()} retrying={booking.isFetching}>
          <Link to="/tickets" className="btn-ghost">
            Find my tickets
          </Link>
        </ErrorState>
      </div>
    )
  }

  const b = booking.data
  return (
    <div className="relative mx-auto max-w-3xl px-4 pt-10 sm:px-6 sm:pt-16">
      {celebrate && <Confetti />}

      {/* Celebration */}
      <motion.div
        initial={{ opacity: 0, scale: 0.96 }}
        animate={{ opacity: 1, scale: 1 }}
        transition={{ duration: 0.5, ease: 'easeOut' }}
        className="relative text-center"
      >
        <div aria-hidden="true" className="absolute inset-x-0 -top-10 -z-10 mx-auto h-48 w-72 rounded-full bg-neon-violet/30 blur-3xl" />
        <div aria-hidden="true" className="mx-auto grid size-16 place-items-center rounded-full bg-gradient-to-br from-neon-cyan via-neon-violet to-neon-pink text-3xl text-night-950 shadow-[0_0_40px_8px_rgb(168_85_247/0.45)]">
          ✓
        </div>
        <h1 className="mt-6 text-4xl font-bold sm:text-5xl">
          You’re <span className="text-gradient">going!</span>
        </h1>
        <p className="mt-3 text-ink-soft">
          {event.data ? (
            <>
              {event.data.artist} · {event.data.name}
            </>
          ) : (
            'Your booking is confirmed.'
          )}
        </p>
      </motion.div>

      {/* Booking details */}
      <section aria-label="Booking details" className="glass mt-10 rounded-3xl p-6">
        {b ? (
          <dl className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
            <Detail label="Seat" value={b.seatId} />
            <Detail label="Paid" value={formatPrice(b.amountCents, b.currency)} />
            <Detail label="Booking" value={b.id.slice(0, 8).toUpperCase()} mono />
            <Detail label="Sent to" value={b.email} truncate />
          </dl>
        ) : (
          <div className="skeleton h-12" aria-busy="true" aria-label="Loading booking" />
        )}
      </section>

      {/* Ticket */}
      <section aria-live="polite" aria-label="Your ticket" className="mt-8">
        <AnimatePresence mode="wait">
          {ticket ? (
            <motion.div
              key="ticket"
              initial={{ opacity: 0, y: 24, rotateX: 18, scale: 0.96 }}
              animate={{ opacity: 1, y: 0, rotateX: 0, scale: 1 }}
              transition={{ type: 'spring', stiffness: 140, damping: 18 }}
              style={{ transformPerspective: 900 }}
            >
              <TicketCard ticket={ticket} />
              <p className="mt-4 text-center text-sm text-ink-soft">
                We’ve emailed it to you too. You can always find it in{' '}
                <Link to="/tickets" className="font-semibold text-neon-cyan hover:underline">
                  My tickets
                </Link>
                .
              </p>
            </motion.div>
          ) : (
            <motion.div key="issuing" exit={{ opacity: 0, scale: 0.98 }} className="glass relative overflow-hidden rounded-3xl p-8 text-center">
              <div className="skeleton absolute inset-0 rounded-none opacity-60" aria-hidden="true" />
              <div className="relative">
                <div aria-hidden="true" className="mx-auto size-10 animate-spin rounded-full border-2 border-white/15 border-t-neon-cyan" />
                <p className="mt-4 font-display text-lg font-semibold">Your ticket is being issued…</p>
                <p className="mt-1 text-sm text-ink-soft">
                  {slow ? 'This is taking a little longer than usual — it will appear here as soon as it’s ready.' : 'This usually takes a few seconds.'}
                </p>
                {tickets.isError && <p className="mt-2 text-xs text-ink-muted">Still checking…</p>}
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      </section>

      <div className="mt-10 flex justify-center gap-3">
        <Link to="/" className="btn-ghost">
          Browse more events
        </Link>
      </div>
    </div>
  )
}

function Detail({ label, value, mono, truncate }: { label: string; value: string; mono?: boolean; truncate?: boolean }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-ink-muted">{label}</dt>
      <dd className={`mt-0.5 font-semibold text-ink ${mono ? 'font-mono' : ''} ${truncate ? 'truncate' : ''}`} title={truncate ? value : undefined}>
        {value}
      </dd>
    </div>
  )
}
