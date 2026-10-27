import { colors, metricColors } from '@opensynapse/ui'
import { useUIStore } from '../../stores/ui-store'

/** Chart colours for the current theme and palette. */
export function useChartTheme() {
  const palette = useUIStore((s) => s.palette)
  const theme = useUIStore((s) => s.theme)
  const resolved = theme === 'light' || (theme === 'system' && document.documentElement.dataset.theme === 'light') ? 'light' : 'dark'
  return { metric: metricColors(palette), chrome: colors.chartChrome[resolved] }
}
