import presets from './colorSchemes.json'
import type { ColorPalette, ColorScheme } from './colorSchemes'

// This function is also embedded in the HTML. Keep it self-contained so the
// browser can restore appearance before the application modules are loaded.
export function applyAppearance(palette: ColorPalette, dark: boolean, id: string) {
  const root = document.documentElement
  root.classList.toggle('dark', dark)
  root.dataset.colorScheme = id
  root.style.colorScheme = dark ? 'dark' : 'light'
  for (const [role, color] of Object.entries(palette)) root.style.setProperty(role, color)
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', palette['--md-sys-color-surface'])
  const favicon = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (favicon) {
    // Retain VoHive's original gradient icon for the original blue palette.
    const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><path d="M16 2 L28 9 L28 23 L16 30 L4 23 L4 9 Z" fill="${palette['--md-sys-color-primary']}"/><path d="M10 10 L16 22 L22 10 L19 10 L16 17 L13 10 Z" fill="${palette['--md-sys-color-on-primary']}"/></svg>`
    favicon.href = id === 'blue' ? '/favicon.svg' : `data:image/svg+xml,${encodeURIComponent(svg)}`
  }
}

function restoreAppearance(schemes: ColorScheme[], apply: typeof applyAppearance) {
  let mode: string | null = null
  let color: string | null = null
  try {
    mode = localStorage.getItem('theme')
    color = localStorage.getItem('color-scheme')
  } catch { /* A blocked store still gets the default palette and system mode. */ }
  const dark = mode === 'dark' || (mode !== 'light' && matchMedia('(prefers-color-scheme: dark)').matches)
  const scheme = schemes.find(scheme => scheme.id === color) || schemes[0]
  apply(dark ? scheme.dark : scheme.light, dark, scheme.id)
}

export function appearanceBootstrap() {
  return `(${restoreAppearance.toString()})(${JSON.stringify(presets)}, ${applyAppearance.toString()});`
}
