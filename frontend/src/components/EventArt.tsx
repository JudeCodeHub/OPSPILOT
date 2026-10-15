import type { ReactNode } from 'react'

const THEMES = ['aurora', 'neon', 'sunset', 'ocean', 'ember', 'prism'] as const
type KnownTheme = (typeof THEMES)[number]

function normalize(theme: string): KnownTheme {
  return (THEMES as readonly string[]).includes(theme) ? (theme as KnownTheme) : 'prism'
}

/**
 * Generated artwork for an event: layered animated gradients plus an SVG
 * motif per theme. No images are downloaded.
 */
export function EventArt({ theme, className = '', children }: { theme: string; className?: string; children?: ReactNode }) {
  const t = normalize(theme)
  return (
    <div className={`art art-${t} ${className}`}>
      <div className="art-layer" aria-hidden="true" />
      {t === 'neon' && <div className="art-neon-grid" aria-hidden="true" />}
      <svg className="art-svg" viewBox="0 0 400 240" preserveAspectRatio="xMidYMid slice" aria-hidden="true">
        {motifs[t]}
      </svg>
      {children && <div className="relative z-10 h-full">{children}</div>}
    </div>
  )
}

// Fixed star positions so the artwork is identical on every render.
const stars = [
  [30, 30], [80, 18], [140, 44], [210, 22], [260, 52], [330, 26], [370, 60], [52, 72], [180, 70], [300, 84], [110, 92], [356, 104],
]

/** A wave 800 units wide (twice the viewBox) so a -50% translate loops seamlessly. */
function wave(y: number): string {
  const segments = Array.from({ length: 8 }, (_, k) => `Q ${25 + k * 100} ${y - 18} ${50 + k * 100} ${y} T ${100 + k * 100} ${y}`)
  return `M0 ${y} ${segments.join(' ')} L800 240 L0 240 Z`
}

const motifs: Record<KnownTheme, ReactNode> = {
  aurora: (
    <g>
      {stars.map(([x, y], i) => (
        <circle key={i} cx={x} cy={y} r={i % 3 === 0 ? 1.4 : 0.9} fill="white" opacity={0.7} />
      ))}
      <path d="M-20 170 C 60 120, 140 200, 220 150 S 360 110, 420 150" fill="none" stroke="rgb(110 231 183 / 0.55)" strokeWidth="2" />
      <path d="M-20 190 C 80 150, 160 220, 240 175 S 360 140, 420 175" fill="none" stroke="rgb(103 232 249 / 0.35)" strokeWidth="1.5" />
      <path d="M0 240 L70 200 L120 220 L190 180 L260 215 L330 190 L400 220 L400 240 Z" fill="rgb(5 11 31 / 0.85)" />
    </g>
  ),
  neon: (
    <g>
      <circle cx="200" cy="110" r="52" fill="url(#neonSun)" />
      <defs>
        <linearGradient id="neonSun" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#fde047" />
          <stop offset="1" stopColor="#f472b6" />
        </linearGradient>
      </defs>
      {[96, 108, 120, 132, 144, 156].map((y, i) => (
        <rect key={y} x="140" y={y} width="120" height={2 + i * 0.8} fill="#2a0845" />
      ))}
    </g>
  ),
  sunset: (
    <g>
      <circle cx="200" cy="150" r="46" fill="rgb(254 240 138 / 0.9)" />
      {[0, 1, 2, 3].map((i) => (
        <rect key={i} x="0" y={176 + i * 9} width="400" height="2" fill="rgb(255 255 255 / 0.18)" />
      ))}
      <path d="M0 240 L0 196 Q 60 176 120 194 T 250 188 T 400 196 L400 240 Z" fill="rgb(43 15 58 / 0.85)" />
    </g>
  ),
  ocean: (
    <g className="art-waves">
      {[0, 1, 2].map((i) => (
        <path
          key={i}
          d={wave(150 + i * 26)}
          fill={`rgb(${[56, 45, 14][i]} ${[189, 212, 116][i]} ${[248, 191, 144][i]} / ${0.22 + i * 0.12})`}
        />
      ))}
    </g>
  ),
  ember: (
    <g>
      {[40, 90, 130, 175, 220, 260, 300, 345, 375].map((x, i) => (
        <circle
          key={x}
          className="art-spark"
          cx={x}
          cy={230}
          r={1.5 + (i % 3)}
          fill={i % 2 ? '#fdba74' : '#fb923c'}
          style={{ animationDelay: `${(i * 0.7) % 6}s`, animationDuration: `${5 + (i % 4)}s` }}
        />
      ))}
    </g>
  ),
  prism: (
    <g fill="none" strokeWidth="1.5">
      <path d="M200 40 L280 180 L120 180 Z" stroke="rgb(255 255 255 / 0.6)" />
      <path d="M200 70 L255 165 L145 165 Z" stroke="rgb(34 211 238 / 0.6)" />
      <path d="M0 120 L170 112" stroke="rgb(255 255 255 / 0.7)" />
      <path d="M236 120 L400 92" stroke="#22d3ee" />
      <path d="M236 124 L400 120" stroke="#a855f7" />
      <path d="M236 128 L400 148" stroke="#f472b6" />
      <path d="M236 132 L400 176" stroke="#fbbf24" />
    </g>
  ),
}
