import {
  argbFromHex, hexFromArgb, Hct, SchemeTonalSpot,
  MaterialDynamicColors as roles, type DynamicColor
} from '@material/material-color-utilities'
import type { ColorPalette } from './colorSchemes'

export const defaultCustomColor = '#65568c'

export function normalizeCustomColor(value: unknown): string | null {
  return typeof value === 'string' && /^#[0-9a-f]{6}$/i.test(value.trim())
    ? value.trim().toLowerCase() : null
}

export function createCustomColorScheme(seed: string) {
  const normalized = normalizeCustomColor(seed)
  if (!normalized) throw new Error('请输入 #RRGGBB 格式的颜色代码。')
  const source = Hct.fromInt(argbFromHex(normalized))
  function palette(dark: boolean): ColorPalette {
    const scheme = new SchemeTonalSpot(source, dark, 0)
    const color = (role: DynamicColor) => hexFromArgb(role.getArgb(scheme))
    return {
      '--md-sys-color-primary': color(roles.primary),
      '--md-sys-color-on-primary': color(roles.onPrimary),
      '--md-sys-color-primary-container': color(roles.primaryContainer),
      '--md-sys-color-on-primary-container': color(roles.onPrimaryContainer),
      '--md-sys-color-secondary': color(roles.secondary),
      '--md-sys-color-on-secondary': color(roles.onSecondary),
      '--md-sys-color-secondary-container': color(roles.secondaryContainer),
      '--md-sys-color-on-secondary-container': color(roles.onSecondaryContainer),
      '--md-sys-color-tertiary': color(roles.tertiary),
      '--md-sys-color-on-tertiary': color(roles.onTertiary),
      '--md-sys-color-tertiary-container': color(roles.tertiaryContainer),
      '--md-sys-color-on-tertiary-container': color(roles.onTertiaryContainer),
      '--md-sys-color-error': color(roles.error),
      '--md-sys-color-on-error': color(roles.onError),
      '--md-sys-color-error-container': color(roles.errorContainer),
      '--md-sys-color-on-error-container': color(roles.onErrorContainer),
      '--md-sys-color-surface': color(roles.surface),
      '--md-sys-color-on-surface': color(roles.onSurface),
      '--md-sys-color-surface-variant': color(roles.surfaceVariant),
      '--md-sys-color-on-surface-variant': color(roles.onSurfaceVariant),
      '--md-sys-color-surface-dim': color(roles.surfaceDim),
      '--md-sys-color-surface-bright': color(roles.surfaceBright),
      '--md-sys-color-surface-container-lowest': color(roles.surfaceContainerLowest),
      '--md-sys-color-surface-container-low': color(roles.surfaceContainerLow),
      '--md-sys-color-surface-container': color(roles.surfaceContainer),
      '--md-sys-color-surface-container-high': color(roles.surfaceContainerHigh),
      '--md-sys-color-surface-container-highest': color(roles.surfaceContainerHighest),
      '--md-sys-color-outline': color(roles.outline),
      '--md-sys-color-outline-variant': color(roles.outlineVariant),
      '--md-sys-color-inverse-surface': color(roles.inverseSurface),
      '--md-sys-color-inverse-on-surface': color(roles.inverseOnSurface),
      '--md-sys-color-inverse-primary': color(roles.inversePrimary),
      '--md-sys-color-surface-tint': color(roles.primary),
      '--md-sys-color-scrim': '#000000',
      '--md-sys-color-shadow': '#000000'
    }
  }
  return { id: 'custom' as const, label: '自定义', light: palette(false), dark: palette(true) }
}
