/**
 * Reads a whole number typed into a field, clamped to [min, max]. Number
 * inputs keep the raw text while editing (so they can be cleared and
 * retyped) and are clamped only where the value is used.
 */
export function clampInt(text: string | number, min: number, max: number, fallback: number): number {
  const n = Math.round(Number(text))
  if (text === '' || !Number.isFinite(n)) return fallback
  return Math.min(max, Math.max(min, n))
}
