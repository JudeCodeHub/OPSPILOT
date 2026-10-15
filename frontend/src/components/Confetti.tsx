import { useState } from 'react'
import { motion, useReducedMotion } from 'motion/react'

const COLORS = ['#22d3ee', '#a855f7', '#f472b6', '#fbbf24', '#a3e635']

interface Piece {
  x: number
  delay: number
  duration: number
  rotate: number
  size: number
  color: string
  drift: number
}

/** A single, short burst of confetti. Renders nothing with reduced motion. */
export function Confetti({ count = 42 }: { count?: number }) {
  const reduce = useReducedMotion()
  const [pieces] = useState<Piece[]>(() =>
    Array.from({ length: count }, (_, i) => ({
      x: Math.random() * 100,
      delay: Math.random() * 0.4,
      duration: 2.2 + Math.random() * 1.6,
      rotate: (Math.random() - 0.5) * 720,
      size: 6 + Math.random() * 6,
      color: COLORS[i % COLORS.length],
      drift: (Math.random() - 0.5) * 120,
    })),
  )
  if (reduce) return null

  return (
    <div aria-hidden="true" className="pointer-events-none fixed inset-0 z-50 overflow-hidden">
      {pieces.map((p, i) => (
        <motion.span
          key={i}
          className="absolute top-0 block rounded-[2px]"
          style={{ left: `${p.x}%`, width: p.size, height: p.size * 0.45, background: p.color }}
          initial={{ y: -20, x: 0, rotate: 0, opacity: 1 }}
          animate={{ y: '105vh', x: p.drift, rotate: p.rotate, opacity: [1, 1, 0] }}
          transition={{ duration: p.duration, delay: p.delay, ease: [0.2, 0.6, 0.4, 1] }}
        />
      ))}
    </div>
  )
}
