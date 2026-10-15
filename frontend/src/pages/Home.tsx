import { useQuery } from '@tanstack/react-query'
import { motion } from 'motion/react'
import { Link } from 'react-router'
import { api, type EventSummary } from '../api'
import { EventArt } from '../components/EventArt'
import { ErrorState } from '../components/ErrorState'
import { dateBadge, formatDateShort, formatPrice, formatTime } from '../lib/format'

function Hero() {
  return (
    <section className="relative overflow-hidden">
      <div aria-hidden="true" className="absolute inset-0 -z-10">
        <div className="absolute -top-32 left-[10%] size-[28rem] animate-drift rounded-full bg-neon-violet/25 blur-3xl" />
        <div className="absolute top-10 right-[5%] size-[22rem] animate-drift-slow rounded-full bg-neon-cyan/20 blur-3xl" />
        <div className="absolute -bottom-20 left-[40%] size-[20rem] animate-drift rounded-full bg-neon-pink/15 blur-3xl" />
      </div>
      <div className="mx-auto max-w-6xl px-4 pt-16 pb-12 sm:px-6 sm:pt-24 sm:pb-16">
        <motion.p
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          className="inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/5 px-3 py-1 text-xs font-medium text-ink-soft"
        >
          <span className="size-1.5 rounded-full bg-neon-lime shadow-[0_0_8px_2px_rgb(163_230_53/0.6)]" aria-hidden="true" />
          Live across Colombo, Kandy &amp; Galle
        </motion.p>
        <motion.h1
          initial={{ opacity: 0, y: 14 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.05 }}
          className="mt-5 max-w-3xl text-4xl leading-[1.05] font-bold sm:text-6xl lg:text-7xl"
        >
          Nights you’ll <span className="text-gradient">still be humming</span> next week.
        </motion.h1>
        <motion.p
          initial={{ opacity: 0, y: 14 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.12 }}
          className="mt-5 max-w-xl text-base text-ink-soft sm:text-lg"
        >
          Pick your seat on a live map, hold it for five minutes, and get your ticket in seconds.
        </motion.p>
      </div>
    </section>
  )
}

function availability(e: EventSummary): { label: string; tone: string } {
  if (e.seatsAvailable === 0) return { label: 'Sold out', tone: 'bg-white/10 text-ink-soft' }
  if (e.seatsTotal > 0 && e.seatsAvailable / e.seatsTotal < 0.2) {
    return { label: 'Selling fast', tone: 'bg-neon-amber/15 text-neon-amber ring-1 ring-neon-amber/30' }
  }
  return { label: 'On sale', tone: 'bg-neon-lime/10 text-neon-lime ring-1 ring-neon-lime/25' }
}

function EventCard({ event }: { event: EventSummary }) {
  const badge = dateBadge(event.startsAt)
  const avail = availability(event)
  return (
    <Link
      to={`/events/${event.id}`}
      className="glass group flex h-full flex-col overflow-hidden rounded-3xl transition duration-300 hover:-translate-y-1.5 hover:border-white/20 hover:shadow-[0_30px_60px_-25px_rgb(168_85_247/0.6)]"
    >
      <EventArt theme={event.theme} className="h-44 sm:h-48">
        <div className="flex h-full items-start justify-between p-4">
          <div className="rounded-xl bg-night-950/60 px-3 py-1.5 text-center backdrop-blur">
            <p className="text-[0.65rem] font-semibold tracking-widest text-ink-soft">{badge.month}</p>
            <p className="font-display text-xl leading-none font-bold">{badge.day}</p>
          </div>
          <span className={`rounded-full px-2.5 py-1 text-xs font-semibold backdrop-blur ${avail.tone}`}>{avail.label}</span>
        </div>
      </EventArt>
      <div className="flex flex-1 flex-col p-5">
        <p className="text-xs font-semibold tracking-[0.2em] text-ink-muted uppercase">{event.artist}</p>
        <h2 className="mt-1 text-xl font-bold text-ink group-hover:text-white">{event.name}</h2>
        <p className="mt-2 text-sm text-ink-soft">
          {event.venue.name} · {event.venue.city}
        </p>
        <p className="text-sm text-ink-muted">
          {formatDateShort(event.startsAt)} · {formatTime(event.startsAt)}
        </p>
        <div className="mt-auto flex items-end justify-between pt-5">
          <p className="text-sm text-ink-soft">
            from <span className="font-display text-lg font-bold text-ink">{formatPrice(event.minPriceCents, event.currency)}</span>
          </p>
          <span className="text-sm font-semibold text-neon-cyan transition group-hover:translate-x-0.5" aria-hidden="true">
            Pick seats →
          </span>
        </div>
      </div>
    </Link>
  )
}

function EventCardSkeleton() {
  return (
    <div className="glass overflow-hidden rounded-3xl" aria-hidden="true">
      <div className="skeleton h-44 rounded-none sm:h-48" />
      <div className="space-y-3 p-5">
        <div className="skeleton h-3 w-24" />
        <div className="skeleton h-6 w-3/4" />
        <div className="skeleton h-4 w-1/2" />
        <div className="skeleton mt-6 h-6 w-32" />
      </div>
    </div>
  )
}

export function Home() {
  const events = useQuery({ queryKey: ['events'], queryFn: api.listEvents })

  return (
    <>
      <Hero />
      <section aria-labelledby="upcoming" className="mx-auto max-w-6xl px-4 sm:px-6">
        <h2 id="upcoming" className="mb-6 text-2xl font-bold sm:text-3xl">
          Upcoming shows
        </h2>

        {events.isPending && (
          <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3" aria-busy="true" aria-label="Loading events">
            {Array.from({ length: 6 }, (_, i) => (
              <EventCardSkeleton key={i} />
            ))}
          </div>
        )}

        {events.isError && <ErrorState error={events.error} onRetry={() => events.refetch()} retrying={events.isFetching} />}

        {events.isSuccess && events.data.length === 0 && (
          <p className="glass rounded-2xl p-8 text-center text-ink-soft">No upcoming shows right now. Check back soon.</p>
        )}

        {events.isSuccess && events.data.length > 0 && (
          <motion.ul
            className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3"
            initial="hidden"
            animate="show"
            variants={{ show: { transition: { staggerChildren: 0.07 } } }}
          >
            {events.data.map((event) => (
              <motion.li
                key={event.id}
                variants={{ hidden: { opacity: 0, y: 18 }, show: { opacity: 1, y: 0, transition: { duration: 0.4, ease: 'easeOut' } } }}
              >
                <EventCard event={event} />
              </motion.li>
            ))}
          </motion.ul>
        )}
      </section>
    </>
  )
}
