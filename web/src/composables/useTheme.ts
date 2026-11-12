import { computed, nextTick, ref, watch } from 'vue'
import { revealTheme } from '../utils/themeTransition'
import { applyAppearance } from '../theme/bootstrap'
import { colorSchemes, findColorScheme, loadPreference, type ColorSchemeId } from '../theme/colorSchemes'

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
const colorId = ref<ColorSchemeId>(findColorScheme(loadPreference('color-scheme')).id)
const colorScheme = computed(() => findColorScheme(colorId.value))
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
  [mode, isDark, palette],
  () => {
    applyAppearance(palette.value, isDark.value, colorScheme.value.id)
    try {
      localStorage.setItem('theme', mode.value)
      localStorage.setItem('color-scheme', colorId.value)
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

async function changeAppearance(nextMode: ThemeMode, nextColor: ColorSchemeId, origin?: { x: number; y: number }) {
  if (isTransitioning.value) return
  if (nextMode === mode.value && nextColor === colorId.value) return
  const nextIsDark = nextMode === 'dark' || (nextMode === 'system' && systemDark.value)
  const applyTheme = async () => {
    mode.value = nextMode
    colorId.value = nextColor
    await nextTick()
  }
  if (!origin || (nextIsDark === isDark.value && nextColor === colorId.value) ||
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

function setColorScheme(next: ColorSchemeId, origin?: { x: number; y: number }) {
  return changeAppearance(mode.value, next, origin)
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
    colorSchemes,
    isTransitioning: computed(() => isTransitioning.value),
    cycle,
    setMode,
    setColorScheme,
  }
}
