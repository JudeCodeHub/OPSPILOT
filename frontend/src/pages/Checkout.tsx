import { useState, type FormEvent, type ReactNode } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AnimatePresence, motion } from 'motion/react'
import { Link, useNavigate, useParams } from 'react-router'
import { api, isApiError } from '../api'
import { EventArt } from '../components/EventArt'
import { ErrorState } from '../components/ErrorState'
import { Spinner } from '../components/Spinner'
import { formatCardInput, formatCountdown, formatDateLong, formatPrice, formatTime } from '../lib/format'
import { holdStore, type StoredHold } from '../lib/storage'
import { useNow } from '../lib/useNow'

/** Below this many ms the countdown turns urgent. */
const URGENT_MS = 60_000

const TEST_CARDS = [
  { label: 'Approve', number: '4242 4242 4242 4242' },
  { label: 'Decline', number: '4000 0000 0000 0002' },
]

export function Checkout() {
  const { holdId = '' } = useParams()
  const [stored] = useState(() => holdStore.get(holdId))

  if (!stored) {
    return (
      <Centered>
        <h1 className="text-2xl font-bold">We can’t find this seat hold</h1>
        <p className="mt-2 text-ink-soft">Holds live in the browser tab where you made them, and only for a few minutes.</p>
        <Link to="/" className="btn-primary mt-6">
          Browse events
        </Link>
      </Centered>
    )
  }
  return <CheckoutForm stored={stored} />
}

function CheckoutForm({ stored }: { stored: StoredHold }) {
  const { hold, event } = stored
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const [cardNumber, setCardNumber] = useState('')
  const [cardName, setCardName] = useState('')

  const checkout = useMutation({
    mutationFn: () => api.checkout(hold.holdId, { cardNumber, cardName: cardName.trim() }),
    onSuccess: (booking) => {
      holdStore.remove(hold.holdId)
      queryClient.setQueryData(['booking', booking.id], booking)
      navigate(`/bookings/${booking.id}`, { state: { celebrate: true } })
    },
  })

  const serverSaysExpired = isApiError(checkout.error) && checkout.error.code === 'HOLD_EXPIRED'
  const now = useNow(250, !serverSaysExpired)
  const remaining = hold.deadline - now
  const expired = serverSaysExpired || remaining <= 0

  const changeSeat = async () => {
    holdStore.remove(hold.holdId)
    try {
      await api.releaseHold(hold.holdId)
    } catch {
      // The hold expires on its own anyway.
    }
    navigate(`/events/${hold.eventId}`)
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!expired && !checkout.isPending) checkout.mutate()
  }

  return (
    <div className="mx-auto max-w-5xl px-4 pt-8 sm:px-6 sm:pt-12">
      <h1 className="text-3xl font-bold sm:text-4xl">Checkout</h1>

      <div className="mt-8 grid gap-6 lg:grid-cols-[1fr_24rem] lg:items-start">
        {/* Payment */}
        <section className="glass order-2 rounded-3xl p-6 sm:p-8 lg:order-1" aria-labelledby="pay">
          <AnimatePresence mode="wait" initial={false}>
            {expired ? (
              <motion.div key="expired" initial={{ opacity: 0, scale: 0.98 }} animate={{ opacity: 1, scale: 1 }} className="py-6 text-center">
                <div aria-hidden="true" className="mx-auto grid size-16 place-items-center rounded-full bg-danger/15 text-3xl">
                  ⏱
                </div>
                <h2 id="pay" className="mt-5 text-2xl font-bold">
                  Your hold has expired
                </h2>
                <p className="mt-2 text-ink-soft">Seat {hold.seatId} has been released so other fans can grab it. Nothing was charged.</p>
                <Link to={`/events/${hold.eventId}`} className="btn-primary mt-6" onClick={() => holdStore.remove(hold.holdId)}>
                  Pick another seat
                </Link>
              </motion.div>
            ) : (
              <motion.form key="form" onSubmit={submit} initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} className="space-y-5">
                <h2 id="pay" className="text-xl font-bold">
                  Payment
                </h2>

                <div className="rounded-2xl border border-neon-cyan/25 bg-neon-cyan/[0.06] p-4 text-sm">
                  <p className="font-semibold text-ink">Demo mode — use a test card</p>
                  <ul className="mt-2 space-y-1.5">
                    {TEST_CARDS.map((c) => (
                      <li key={c.number} className="flex items-center justify-between gap-3">
                        <span className="text-ink-soft">
                          {c.label}: <code className="font-mono text-ink">{c.number}</code>
                        </span>
                        <button type="button" className="rounded-md px-2 py-1 text-xs font-semibold text-neon-cyan hover:bg-white/5" onClick={() => setCardNumber(c.number)}>
                          Use
                        </button>
                      </li>
                    ))}
                  </ul>
                </div>

                <div>
                  <label htmlFor="card-number" className="mb-1.5 block text-sm font-medium text-ink-soft">
                    Card number
                  </label>
                  <input
                    id="card-number"
                    className="input font-mono tracking-wider"
                    inputMode="numeric"
                    autoComplete="cc-number"
                    placeholder="1234 5678 9012 3456"
                    required
                    value={cardNumber}
                    onChange={(e) => setCardNumber(formatCardInput(e.target.value))}
                  />
                </div>
                <div>
                  <label htmlFor="card-name" className="mb-1.5 block text-sm font-medium text-ink-soft">
                    Name on card
                  </label>
                  <input
                    id="card-name"
                    className="input"
                    autoComplete="cc-name"
                    placeholder="Your name"
                    required
                    value={cardName}
                    onChange={(e) => setCardName(e.target.value)}
                  />
                </div>

                <button type="submit" className="btn-primary w-full py-3.5 text-base" disabled={checkout.isPending}>
                  {checkout.isPending ? <Spinner label="Processing payment…" /> : `Pay ${formatPrice(hold.priceCents, hold.currency)}`}
                </button>

                {checkout.isError && (
                  <ErrorState error={checkout.error} compact onRetry={() => checkout.mutate()} retrying={checkout.isPending} />
                )}

                <button type="button" onClick={changeSeat} className="w-full text-center text-sm text-ink-soft hover:text-ink" disabled={checkout.isPending}>
                  Change seat
                </button>
              </motion.form>
            )}
          </AnimatePresence>
        </section>

        {/* Summary + countdown */}
        <aside className="order-1 space-y-4 lg:order-2" aria-label="Order summary">
          <div className="glass overflow-hidden rounded-3xl">
            <EventArt theme={event.theme} className="h-28" />
            <div className="p-6">
              <p className="text-xs font-semibold tracking-[0.25em] text-ink-muted uppercase">{event.artist}</p>
              <p className="mt-1 text-xl font-bold">{event.name}</p>
              <p className="mt-2 text-sm text-ink-soft">
                {formatDateLong(event.startsAt)} · {formatTime(event.startsAt)}
              </p>
              <p className="text-sm text-ink-soft">
                {event.venue.name}, {event.venue.city}
              </p>
              <dl className="mt-5 space-y-2 border-t border-white/10 pt-4 text-sm">
                <div className="flex justify-between">
                  <dt className="text-ink-soft">Seat</dt>
                  <dd className="font-semibold">
                    {hold.seatId} · {stored.section}
                  </dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-ink-soft">Tickets sent to</dt>
                  <dd className="max-w-[60%] truncate font-semibold">{stored.email}</dd>
                </div>
                <div className="flex justify-between border-t border-white/10 pt-3 text-base">
                  <dt className="font-semibold">Total</dt>
                  <dd className="font-display font-bold">{formatPrice(hold.priceCents, hold.currency)}</dd>
                </div>
              </dl>
            </div>
          </div>

          {!expired && <Countdown remaining={remaining} total={hold.ttlMs} />}
        </aside>
      </div>
    </div>
  )
}

function Countdown({ remaining, total }: { remaining: number; total: number }) {
  const urgent = remaining < URGENT_MS
  const pct = Math.max(0, Math.min(100, (remaining / Math.max(total, 1)) * 100))
  return (
    <div
      className={`glass rounded-3xl p-5 transition-colors ${urgent ? 'border-danger/40 bg-danger/[0.06]' : ''}`}
      role="timer"
      aria-label={`Seat held for ${formatCountdown(remaining)}`}
    >
      <div className="flex items-center justify-between">
        <p className="text-sm text-ink-soft">{urgent ? 'Hurry — your hold ends soon' : 'Seat held for'}</p>
        <p className={`font-display text-3xl font-bold tabular-nums ${urgent ? 'animate-pulse-urgent text-danger' : 'text-ink'}`}>{formatCountdown(remaining)}</p>
      </div>
      <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-white/10" aria-hidden="true">
        <div
          className={`h-full rounded-full transition-[width] duration-300 ${urgent ? 'bg-danger' : 'bg-gradient-to-r from-neon-cyan to-neon-violet'}`}
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  )
}

function Centered({ children }: { children: ReactNode }) {
  return <div className="mx-auto max-w-lg px-4 py-24 text-center">{children}</div>
}
