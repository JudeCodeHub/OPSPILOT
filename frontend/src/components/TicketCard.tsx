import { useMemo } from 'react'
import { EventArt } from './EventArt'
import { PATTERN_SIZE, patternFor } from '../lib/pattern'
import { formatDateShort, formatTime } from '../lib/format'

export interface TicketView {
  code: string
  eventName: string
  artist: string
  startsAt: string
  theme: string
  venueName: string
  venueCity: string
  seatId: string
  section: string
}

/** QR-style pattern generated from the ticket code (decorative, rendered locally). */
export function TicketPattern({ code, className = '' }: { code: string; className?: string }) {
  const { cells, finders } = useMemo(() => patternFor(code), [code])
  return (
    <svg
      viewBox={`-1 -1 ${PATTERN_SIZE + 2} ${PATTERN_SIZE + 2}`}
      role="img"
      aria-label={`Ticket pattern for ${code}`}
      className={`rounded-lg bg-white ${className}`}
      shapeRendering="crispEdges"
    >
      {cells.map(([x, y]) => (
        <rect key={`${x}-${y}`} x={x} y={y} width="1" height="1" fill="#0c0a1a" />
      ))}
      {finders.map(([x, y]) => (
        <g key={`${x}-${y}`} fill="#0c0a1a">
          <rect x={x} y={y} width="7" height="7" />
          <rect x={x + 1} y={y + 1} width="5" height="5" fill="white" />
          <rect x={x + 2} y={y + 2} width="3" height="3" />
        </g>
      ))}
    </svg>
  )
}

/** Notch: a circle in the page colour that "cuts" into the ticket edge. */
function Notch({ className }: { className: string }) {
  return <span aria-hidden="true" className={`absolute size-6 rounded-full bg-night-950 ${className}`} />
}

/** A stylised ticket: artwork + details, a perforated edge, and a stub with the code. */
export function TicketCard({ ticket }: { ticket: TicketView }) {
  return (
    <article
      aria-label={`Ticket ${ticket.code} for ${ticket.eventName}`}
      className="relative overflow-hidden rounded-3xl border border-white/10 bg-night-800 shadow-[0_30px_80px_-30px_rgb(168_85_247/0.55)] sm:grid sm:grid-cols-[1fr_auto]"
    >
      {/* Main part */}
      <div className="relative">
        {/* .art sets position: relative, so position the wrapper rather than the art itself. */}
        <div className="h-28 sm:absolute sm:inset-0 sm:h-auto sm:opacity-30">
          <EventArt theme={ticket.theme} className="h-full" />
        </div>
        <div className="relative z-10 p-5 sm:p-6">
          <p className="text-xs font-semibold tracking-[0.25em] text-ink-soft uppercase">{ticket.artist}</p>
          <h3 className="mt-1 text-xl font-bold text-ink sm:text-2xl">{ticket.eventName}</h3>
          <dl className="mt-4 grid grid-cols-3 gap-3 text-sm">
            <div>
              <dt className="text-xs text-ink-muted">Date</dt>
              <dd className="font-medium text-ink">{formatDateShort(ticket.startsAt)}</dd>
            </div>
            <div>
              <dt className="text-xs text-ink-muted">Doors</dt>
              <dd className="font-medium text-ink">{formatTime(ticket.startsAt)}</dd>
            </div>
            <div>
              <dt className="text-xs text-ink-muted">Seat</dt>
              <dd className="font-medium text-ink">
                {ticket.seatId}
                {ticket.section && <span className="ml-1 text-xs text-ink-soft">{ticket.section}</span>}
              </dd>
            </div>
          </dl>
          <p className="mt-3 text-sm text-ink-soft">
            {ticket.venueName}, {ticket.venueCity}
          </p>
        </div>
      </div>

      {/* Perforation + stub */}
      <div className="relative flex items-center gap-4 border-t-2 border-dashed border-white/15 p-5 sm:flex-col sm:justify-center sm:border-t-0 sm:border-l-2 sm:px-7">
        <Notch className="-top-3 -left-3 sm:-top-3 sm:-left-3" />
        <Notch className="-top-3 -right-3 sm:top-auto sm:-bottom-3 sm:-left-3 sm:right-auto" />
        <TicketPattern code={ticket.code} className="size-24 shrink-0 sm:size-28" />
        <div className="sm:text-center">
          <p className="text-xs text-ink-muted">Ticket code</p>
          <p className="font-mono text-base font-semibold tracking-wider text-ink">{ticket.code}</p>
        </div>
      </div>
    </article>
  )
}
