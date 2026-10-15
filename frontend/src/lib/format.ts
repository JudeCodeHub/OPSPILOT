const priceFormatters = new Map<string, Intl.NumberFormat>()

/** Formats cents as a price, e.g. 750000 LKR → "LKR 7,500". */
export function formatPrice(cents: number, currency = 'LKR'): string {
  let fmt = priceFormatters.get(currency)
  if (!fmt) {
    fmt = new Intl.NumberFormat('en-LK', { style: 'currency', currency, currencyDisplay: 'code', maximumFractionDigits: 0 })
    priceFormatters.set(currency, fmt)
  }
  // Intl puts a non-breaking space after the code; keep it.
  return fmt.format(cents / 100)
}

// Concerts are in Sri Lanka, so show their local time wherever the visitor is.
const TZ = 'Asia/Colombo'

const dateLong = new Intl.DateTimeFormat('en-GB', {
  weekday: 'long', day: 'numeric', month: 'long', year: 'numeric', timeZone: TZ,
})
const dateShort = new Intl.DateTimeFormat('en-GB', { weekday: 'short', day: 'numeric', month: 'short', timeZone: TZ })
const time = new Intl.DateTimeFormat('en-GB', { hour: 'numeric', minute: '2-digit', hour12: true, timeZone: TZ })
const monthDay = new Intl.DateTimeFormat('en-GB', { month: 'short', day: '2-digit', timeZone: TZ })

export const formatDateLong = (iso: string) => dateLong.format(new Date(iso))
export const formatDateShort = (iso: string) => dateShort.format(new Date(iso))
export const formatTime = (iso: string) => time.format(new Date(iso)).toUpperCase()

/** Parts for a calendar-style date badge: { month: "NOV", day: "03" }. */
export function dateBadge(iso: string): { month: string; day: string } {
  const parts = monthDay.formatToParts(new Date(iso))
  return {
    month: (parts.find((p) => p.type === 'month')?.value ?? '').toUpperCase(),
    day: parts.find((p) => p.type === 'day')?.value ?? '',
  }
}

/** "4:05" from milliseconds. */
export function formatCountdown(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000))
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`
}

/** Groups a card number in fours as the user types. */
export function formatCardInput(value: string): string {
  return value.replace(/\D/g, '').slice(0, 19).replace(/(\d{4})(?=\d)/g, '$1 ')
}
