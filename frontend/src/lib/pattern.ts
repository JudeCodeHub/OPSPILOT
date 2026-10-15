/**
 * Builds a QR-style square pattern from a ticket code, entirely in the
 * browser. It is decorative (not a scannable QR code) but deterministic: the
 * same code always gives the same pattern.
 */
export const PATTERN_SIZE = 25

/** FNV-1a hash of a string, used to seed the generator. */
function hash(s: string): number {
  let h = 0x811c9dc5
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 0x01000193)
  }
  return h >>> 0
}

/** Small deterministic PRNG (mulberry32). */
function rng(seed: number): () => number {
  let a = seed
  return () => {
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

/** True for cells inside one of the three 7×7 corner "finder" squares (plus a 1-cell margin). */
function inFinder(x: number, y: number): boolean {
  const n = PATTERN_SIZE
  const near = (v: number, start: number) => v >= start && v < start + 8
  return (near(x, 0) && near(y, 0)) || (near(x, n - 8) && near(y, 0)) || (near(x, 0) && near(y, n - 8))
}

export interface Pattern {
  /** Data cells that are filled, as [x, y]. */
  cells: Array<[number, number]>
  /** Top-left corners of the finder squares. */
  finders: Array<[number, number]>
}

export function patternFor(code: string): Pattern {
  const next = rng(hash(code))
  const cells: Array<[number, number]> = []
  for (let y = 0; y < PATTERN_SIZE; y++) {
    for (let x = 0; x < PATTERN_SIZE; x++) {
      if (!inFinder(x, y) && next() < 0.48) cells.push([x, y])
    }
  }
  return { cells, finders: [[0, 0], [PATTERN_SIZE - 7, 0], [0, PATTERN_SIZE - 7]] }
}
