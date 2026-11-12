import { computed, nextTick, ref, watch } from 'vue'
import { revealTheme } from '../utils/themeTransition'
import { applyAppearance } from '../theme/bootstrap'
import { colorSchemes, findColorScheme, loadPreference, type AppearanceColorId } from '../theme/colorSchemes'
import { createCustomColorScheme, defaultCustomColor, normalizeCustomColor } from '../theme/customColor'

export type ThemeMode = 'system' | 'light' | 'dark'
const modes: ThemeMode[] = ['system', 'light', 'dark']

function loadTheme(): ThemeMode {
  try {
    const stored = localStorage.getItem('theme') as ThemeMode
    if (modes.includes(stored)) return stored
  } catch {
    /* Storage can be unavailable in private browsing. */
  }
  return 'system'
}

const mode = ref<ThemeMode>(loadTheme())
const storedCustomColor = normalizeCustomColor(loadPreference('custom-color'))
const customColor = ref(storedCustomColor || defaultCustomColor)
const storedColorId = loadPreference('color-scheme')
const colorId = ref<AppearanceColorId>(storedColorId === 'custom' && storedCustomColor ? 'custom' : findColorScheme(storedColorId).id)
const customScheme = computed(() => createCustomColorScheme(customColor.value))
const availableColorSchemes = computed(() => [...colorSchemes, customScheme.value])
const colorScheme = computed(() => colorId.value === 'custom' ? customScheme.value : findColorScheme(colorId.value))
const isTransitioning = ref(false)
const media = window.matchMedia('(prefers-color-scheme: dark)')
const systemDark = ref(media.matches)
// One listener for the app lifetime, shared by both layouts and chart consumers.
media.addEventListener('change', (event) => {
  systemDark.value = event.matches
})
const isDark = computed(
  () => mode.value === 'dark' || (mode.value === 'system' && systemDark.value),
)
const palette = computed(() => isDark.value ? colorScheme.value.dark : colorScheme.value.light)
watch(
  [mode, isDark, palette, customColor],
  () => {
    applyAppearance(palette.value, isDark.value, colorScheme.value.id)
    try {
      localStorage.setItem('theme', mode.value)
      localStorage.setItem('color-scheme', colorId.value)
      localStorage.setItem('custom-color', customColor.value)
      if (colorId.value === 'custom') {
        // A derived cache restores custom colors before the algorithm loads.
        // The HTML bootstrap validates its schema and matching seed first.
        localStorage.setItem('custom-palette', JSON.stringify({ version: 1, seed: customColor.value, light: customScheme.value.light, dark: customScheme.value.dark }))
      }
    } catch {
      /* Keep the in-memory preference. */
    }
  },
  { immediate: true, flush: 'sync' },
)

export function themeOrigin(event: Event) {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 }
}

async function changeAppearance(nextMode: ThemeMode, nextColor: AppearanceColorId, origin?: { x: number; y: number }, seed = customColor.value) {
  if (isTransitioning.value) return
  if (nextMode === mode.value && nextColor === colorId.value && seed === customColor.value) return
  const nextIsDark = nextMode === 'dark' || (nextMode === 'system' && systemDark.value)
  const applyTheme = async () => {
    mode.value = nextMode
    colorId.value = nextColor
    customColor.value = seed
    await nextTick()
  }
  const sameAppearance = nextIsDark === isDark.value && nextColor === colorId.value && (nextColor !== 'custom' || seed === customColor.value)
  if (!origin || sameAppearance ||
      typeof document.startViewTransition !== 'function' ||
      window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
    await applyTheme()
    return
  }

  isTransitioning.value = true
  try {
    await revealTheme(origin, applyTheme)
  } finally {
    isTransitioning.value = false
  }
}

function setMode(next: ThemeMode, origin?: { x: number; y: number }) {
  return changeAppearance(next, colorId.value, origin)
}

function setColorScheme(next: AppearanceColorId, origin?: { x: number; y: number }) {
  return changeAppearance(mode.value, next, origin)
}

function setCustomColor(value: string, origin?: { x: number; y: number }) {
  const seed = normalizeCustomColor(value)
  if (seed) return changeAppearance(mode.value, 'custom', origin, seed)
}

function resetColors(origin?: { x: number; y: number }) {
  return changeAppearance(mode.value, 'blue', origin, defaultCustomColor)
}

function cycle(origin?: { x: number; y: number }) {
  return setMode(modes[(modes.indexOf(mode.value) + 1) % modes.length]!, origin)
}

export function useTheme() {
  return {
    mode: computed(() => mode.value),
    isDark,
    palette,
    colorScheme,
    colorSchemes: availableColorSchemes,
    customColor: computed(() => customColor.value),
    isTransitioning: computed(() => isTransitioning.value),
    cycle,
    setMode,
    setColorScheme,
    setCustomColor,
    resetColors,
  }
}
