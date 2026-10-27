/** Parses k6 durations to seconds: "30s", "2m", "1h30m", "500ms", or a bare number. */
export function parseDurationToSeconds(d: string | undefined): number {
  if (!d) return 0
  if (/^\d+(\.\d+)?$/.test(d.trim())) return Number(d)
  let total = 0
  for (const [, n, unit] of d.matchAll(/(\d+(?:\.\d+)?)(ms|h|m|s)/g)) {
    total += Number(n) * ({ h: 3600, m: 60, s: 1, ms: 0.001 } as Record<string, number>)[unit]
  }
  return total
}
