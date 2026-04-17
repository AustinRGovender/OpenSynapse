/**
 * Parse a k6 duration string ("30s", "2m", "4h") to integer seconds.
 */
export function parseDurationToSeconds(d: string): number {
  const match = d.match(/^(\d+(?:\.\d+)?)(s|m|h)$/)
  if (!match) return 0
  const value = parseFloat(match[1])
  switch (match[2]) {
    case 'h':
      return Math.round(value * 3600)
    case 'm':
      return Math.round(value * 60)
    case 's':
    default:
      return Math.round(value)
  }
}

/**
 * Convert seconds to a human-readable duration string.
 */
export function formatSecondsToDuration(s: number): string {
  if (s >= 3600 && s % 3600 === 0) return `${s / 3600}h`
  if (s >= 60 && s % 60 === 0) return `${s / 60}m`
  return `${s}s`
}
