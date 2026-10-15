import { useMemo, useRef, useState, type KeyboardEvent } from 'react'
import type { Seat, Section } from '../api'
import { formatPrice } from '../lib/format'

interface SeatMapProps {
  seats: Seat[]
  selectedId: string | null
  onSelect: (seat: Seat) => void
}

/** Seats per row before the centre aisle. */
const AISLE_AFTER = 6

const statusLabel: Record<Seat['status'], string> = {
  available: 'available',
  held: 'on hold by another fan',
  sold: 'sold',
}

/**
 * Interactive seat map. Keyboard: one Tab stop for the whole grid (roving
 * tabindex), arrow keys move between seats, Home/End jump to the row ends,
 * Enter or Space selects. Unavailable seats can be focused (so their state is
 * announced) but not selected.
 */
export function SeatMap({ seats, selectedId, onSelect }: SeatMapProps) {
  const rows = useMemo(() => {
    const byRow = new Map<string, Seat[]>()
    for (const seat of seats) {
      const list = byRow.get(seat.row) ?? []
      list.push(seat)
      byRow.set(seat.row, list)
    }
    return [...byRow.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([row, list]) => ({ row, seats: list.sort((a, b) => a.number - b.number) }))
  }, [seats])

  // Focus position [row, column] for the roving tabindex.
  const [focus, setFocus] = useState<[number, number]>([0, 0])
  const buttons = useRef(new Map<string, HTMLButtonElement>())

  const moveTo = (r: number, c: number) => {
    const row = Math.max(0, Math.min(rows.length - 1, r))
    const col = Math.max(0, Math.min((rows[row]?.seats.length ?? 1) - 1, c))
    setFocus([row, col])
    const seat = rows[row]?.seats[col]
    if (seat) buttons.current.get(seat.seatId)?.focus()
  }

  const onKeyDown = (e: KeyboardEvent, r: number, c: number) => {
    const keys: Record<string, () => void> = {
      ArrowRight: () => moveTo(r, c + 1),
      ArrowLeft: () => moveTo(r, c - 1),
      ArrowDown: () => moveTo(r + 1, c),
      ArrowUp: () => moveTo(r - 1, c),
      Home: () => moveTo(r, 0),
      End: () => moveTo(r, Infinity),
    }
    const action = keys[e.key]
    if (action) {
      e.preventDefault()
      action()
    }
  }

  return (
    <div className="w-full">
      {/* Stage */}
      <div className="mx-auto mb-8 max-w-md" aria-hidden="true">
        <div className="h-2 rounded-full bg-gradient-to-r from-neon-cyan via-neon-violet to-neon-pink shadow-[0_0_30px_6px_rgb(168_85_247/0.45)]" />
        <p className="mt-2 text-center text-[0.65rem] font-semibold tracking-[0.4em] text-ink-muted">STAGE</p>
      </div>

      <div role="grid" aria-label="Seat map" aria-rowcount={rows.length} className="mx-auto flex w-fit flex-col gap-1.5 sm:gap-2">
        {rows.map(({ row, seats: rowSeats }, r) => (
          <div role="row" aria-rowindex={r + 1} key={row} className="flex items-center gap-1 sm:gap-1.5">
            <span aria-hidden="true" className="w-4 text-center text-[0.65rem] font-semibold text-ink-muted sm:w-6">{row}</span>
            {rowSeats.map((seat, c) => {
              const selected = seat.seatId === selectedId
              const isFocusStop = focus[0] === r && focus[1] === c
              const label = `Seat ${seat.seatId}, ${seat.section}, ${formatPrice(seat.priceCents, seat.currency)}, ${statusLabel[seat.status]}`
              return (
                <div
                  role="gridcell"
                  aria-selected={selected}
                  key={seat.seatId}
                  className={`group relative ${c === AISLE_AFTER ? 'ml-2.5 sm:ml-4' : ''}`}
                >
                  <button
                    type="button"
                    ref={(el) => {
                      if (el) buttons.current.set(seat.seatId, el)
                      else buttons.current.delete(seat.seatId)
                    }}
                    tabIndex={isFocusStop ? 0 : -1}
                    aria-label={label}
                    aria-disabled={seat.status !== 'available'}
                    className="seat"
                    data-section={seat.section}
                    data-status={seat.status}
                    data-selected={selected}
                    onFocus={() => setFocus([r, c])}
                    onKeyDown={(e) => onKeyDown(e, r, c)}
                    onClick={() => seat.status === 'available' && onSelect(seat)}
                  >
                    <span aria-hidden="true" className="hidden sm:inline">{seat.status === 'sold' ? '×' : seat.number}</span>
                  </button>
                  <Tooltip seat={seat} />
                </div>
              )
            })}
            <span aria-hidden="true" className="w-4 text-center text-[0.65rem] font-semibold text-ink-muted sm:w-6">{row}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

function Tooltip({ seat }: { seat: Seat }) {
  return (
    <div
      aria-hidden="true"
      className="pointer-events-none absolute bottom-full left-1/2 z-20 mb-2 -translate-x-1/2 whitespace-nowrap rounded-lg border border-white/10 bg-night-800/95 px-3 py-2 text-xs opacity-0 shadow-xl backdrop-blur transition-opacity duration-150 group-hover:opacity-100 group-focus-within:opacity-100"
    >
      <p className="font-display font-semibold text-ink">Seat {seat.seatId}</p>
      <p className="text-ink-soft">
        {seat.section} · {formatPrice(seat.priceCents, seat.currency)}
      </p>
      {seat.status !== 'available' && <p className="text-ink-muted capitalize">{seat.status === 'held' ? 'On hold' : 'Sold'}</p>}
    </div>
  )
}

const sectionOrder: Section[] = ['VIP', 'Premium', 'Standard']

/** Legend: one chip per section (with its price) and one per seat state. */
export function SeatLegend({ seats }: { seats: Seat[] }) {
  const prices = new Map<Section, Seat>()
  for (const s of seats) if (!prices.has(s.section)) prices.set(s.section, s)

  return (
    <div className="flex flex-col gap-4 text-xs text-ink-soft sm:flex-row sm:flex-wrap sm:items-center sm:justify-between">
      <ul className="flex flex-wrap gap-x-5 gap-y-2" aria-label="Sections">
        {sectionOrder.map((section) => {
          const seat = prices.get(section)
          if (!seat) return null
          return (
            <li key={section} className="flex items-center gap-2">
              <span className="seat w-4!" data-section={section} data-status="available" aria-hidden="true" />
              <span>
                <span className="font-semibold text-ink">{section}</span> · {formatPrice(seat.priceCents, seat.currency)}
              </span>
            </li>
          )
        })}
      </ul>
      <ul className="flex flex-wrap gap-x-5 gap-y-2" aria-label="Seat states">
        <li className="flex items-center gap-2"><span className="seat w-4!" data-status="available" aria-hidden="true" />Available</li>
        <li className="flex items-center gap-2"><span className="seat w-4!" data-status="available" data-selected="true" aria-hidden="true" />Selected</li>
        <li className="flex items-center gap-2"><span className="seat w-4!" data-status="held" aria-hidden="true" />On hold</li>
        <li className="flex items-center gap-2"><span className="seat w-4!" data-status="sold" aria-hidden="true" />Sold</li>
      </ul>
    </div>
  )
}
