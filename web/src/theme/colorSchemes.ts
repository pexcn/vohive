import presets from './colorSchemes.json'

export type ColorPalette = Record<keyof typeof presets[0]['light'], string>
export type ColorSchemeId = 'blue' | 'green'
export type AppearanceColorId = ColorSchemeId | 'custom'
export type ColorScheme = {
  id: string
  label: string
  light: ColorPalette
  dark: ColorPalette
}

export const colorSchemes = presets as (ColorScheme & { id: ColorSchemeId })[]

export function findColorScheme(id: unknown): ColorScheme & { id: ColorSchemeId } {
  return colorSchemes.find(scheme => scheme.id === id) || colorSchemes[0]
}

export function loadPreference(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}
