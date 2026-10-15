import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AnimatePresence, motion } from 'motion/react'
import { Link, useNavigate, useParams } from 'react-router'
import { api, isApiError, type Seat } from '../api'
import { EventArt } from '../components/EventArt'
import { ErrorState } from '../components/ErrorState'
import { SeatLegend, SeatMap } from '../components/SeatMap'
import { Spinner } from '../components/Spinner'
import { formatDateLong, formatPrice, formatTime } from '../lib/format'
import { holdStore, lastEmail } from '../lib/storage'

/** How often the seat map refreshes, so holds and sales by others show up. */
const SEAT_POLL_MS = 4000

export function EventDetail() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const event = useQuery({ queryKey: ['event', id], queryFn: () => api.getEvent(id) })
  const seats = useQuery({
    queryKey: ['seats', id],
    queryFn: () => api.getSeats(id),
    refetchInterval: SEAT_POLL_MS,
    enabled: event.isSuccess,
  })

  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [email, setEmail] = useState(lastEmail.get)

  const selected: Seat | undefined = seats.data?.seats.find((s) => s.seatId === selectedId)
  // The selection is dropped as soon as a refresh shows someone else took the seat.
  const lostSelection = selected !== undefined && selected.status !== 'available'
  const activeSeat = lostSelection ? undefined : selected

  const hold = useMutation({
    mutationFn: (seat: Seat) => api.createHold({ eventId: id, seatId: seat.seatId, email: email.trim() }),
    onSuccess: (h, seat) => {
      lastEmail.set(email.trim())
      if (event.data) {
        holdStore.set({ hold: h, email: email.trim(), event: event.data, section: seat.section })
      }
      navigate(`/checkout/${h.holdId}`)
    },
    onError: (err) => {
      if (isApiError(err) && err.code === 'SEAT_UNAVAILABLE') {
        setSelectedId(null)
        void queryClient.invalidateQueries({ queryKey: ['seats', id] })
      }
    },
  })

  const select = (seat: Seat) => {
    hold.reset()
    setSelectedId(seat.seatId === selectedId ? null : seat.seatId)
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (activeSeat) hold.mutate(activeSeat)
  }

  if (event.isError) {
    return (
      <div className="mx-auto max-w-2xl px-4 py-16">
        <ErrorState error={event.error} onRetry={() => event.refetch()} retrying={event.isFetching}>
          <Link to="/" className="btn-ghost">
            Back to events
          </Link>
        </ErrorState>
      </div>
    )
  }

  const e = event.data
  return (
    <div className="mx-auto max-w-6xl px-4 pt-6 sm:px-6 sm:pt-10">
      <Link to="/" className="text-sm text-ink-soft hover:text-ink">
        ← All events
      </Link>

      {/* Header */}
      <section className="mt-4 overflow-hidden rounded-3xl border border-white/10">
        {e ? (
          <EventArt theme={e.theme} className="h-64 sm:h-80">
            <div className="flex h-full flex-col justify-end bg-gradient-to-t from-night-950/90 via-night-950/30 to-transparent p-6 sm:p-10">
              <p className="text-xs font-semibold tracking-[0.3em] text-ink-soft uppercase">{e.artist}</p>
              <h1 className="mt-2 text-3xl font-bold sm:text-5xl">{e.name}</h1>
              <p className="mt-3 text-sm text-ink-soft sm:text-base">
                {formatDateLong(e.startsAt)} · {formatTime(e.startsAt)} · {e.venue.name}, {e.venue.city}
              </p>
            </div>
          </EventArt>
        ) : (
          <div className="skeleton h-64 rounded-none sm:h-80" aria-busy="true" aria-label="Loading event" />
        )}
      </section>

      {e && <p className="mt-6 max-w-3xl text-ink-soft">{e.description}</p>}

      <div className="mt-8 grid gap-6 lg:grid-cols-[1fr_22rem] lg:items-start">
        {/* Seat map */}
        <section aria-labelledby="choose-seat" className="glass rounded-3xl p-4 sm:p-8">
          <div className="mb-6 flex items-center justify-between gap-4">
            <h2 id="choose-seat" className="text-xl font-bold">
              Choose your seat
            </h2>
            <LiveIndicator ok={!seats.isError} updating={seats.isFetching} />
          </div>

          {seats.isPending && !seats.isError && <div className="skeleton mx-auto h-80 max-w-xl" aria-busy="true" aria-label="Loading seat map" />}
          {seats.isError && !seats.data && <ErrorState error={seats.error} onRetry={() => seats.refetch()} retrying={seats.isFetching} compact />}

          {seats.data && (
            <>
              <SeatMap seats={seats.data.seats} selectedId={activeSeat?.seatId ?? null} onSelect={select} />
              <div className="mt-8 border-t border-white/5 pt-5">
                <SeatLegend seats={seats.data.seats} />
              </div>
            </>
          )}
        </section>

        {/* Selection + hold */}
        <aside className="glass rounded-3xl p-6 lg:sticky lg:top-24" aria-labelledby="your-seat">
          <h2 id="your-seat" className="text-lg font-bold">
            Your seat
          </h2>

          <AnimatePresence mode="wait" initial={false}>
            {activeSeat ? (
              <motion.div key={activeSeat.seatId} initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0 }}>
                <div className="mt-4 flex items-baseline justify-between">
                  <p className="font-display text-4xl font-bold">{activeSeat.seatId}</p>
                  <p className="font-display text-xl font-bold">{formatPrice(activeSeat.priceCents, activeSeat.currency)}</p>
                </div>
                <p className="text-sm text-ink-soft">
                  {activeSeat.section} · Row {activeSeat.row}, seat {activeSeat.number}
                </p>
              </motion.div>
            ) : (
              <motion.p key="none" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} className="mt-3 text-sm text-ink-soft">
                {lostSelection
                  ? `Seat ${selectedId} was just taken by someone else. Pick another one.`
                  : 'Select an available seat on the map. Use the arrow keys to move between seats.'}
              </motion.p>
            )}
          </AnimatePresence>

          <form onSubmit={submit} className="mt-6 space-y-4">
            <div>
              <label htmlFor="email" className="mb-1.5 block text-sm font-medium text-ink-soft">
                Email for your ticket
              </label>
              <input
                id="email"
                type="email"
                required
                autoComplete="email"
                className="input"
                placeholder="you@example.com"
                value={email}
                onChange={(ev) => setEmail(ev.target.value)}
              />
            </div>
            <button type="submit" className="btn-primary w-full" disabled={!activeSeat || hold.isPending}>
              {hold.isPending ? <Spinner label="Holding seat…" /> : 'Hold seat'}
            </button>
            <p className="text-xs text-ink-muted">We’ll hold it for 5 minutes while you check out.</p>
          </form>

          {hold.isError && (
            <div className="mt-5">
              <ErrorState error={hold.error} compact onRetry={() => activeSeat && hold.mutate(activeSeat)} retrying={hold.isPending} />
            </div>
          )}
        </aside>
      </div>
    </div>
  )
}

function LiveIndicator({ ok, updating }: { ok: boolean; updating: boolean }) {
  return (
    <span className="inline-flex items-center gap-2 text-xs text-ink-muted" aria-live="polite">
      <span
        aria-hidden="true"
        className={`size-2 rounded-full ${ok ? 'bg-neon-lime shadow-[0_0_8px_2px_rgb(163_230_53/0.5)]' : 'bg-neon-amber'} ${updating ? 'animate-pulse' : ''}`}
      />
      {ok ? 'Live' : 'Live updates paused'}
    </span>
  )
}
