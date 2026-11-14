import { computed, nextTick, ref, watch } from 'vue'
import { revealTheme } from '../utils/themeTransition'

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
watch(
  [mode, isDark],
  () => {
    document.documentElement.classList.toggle('dark', isDark.value)
    try {
      localStorage.setItem('theme', mode.value)
    } catch {
      /* Keep the in-memory preference. */
    }
  },
  { immediate: true, flush: 'sync' },
)

async function cycle(origin?: { x: number; y: number }) {
  if (isTransitioning.value) return
  const nextMode = modes[(modes.indexOf(mode.value) + 1) % modes.length]!
  const nextIsDark = nextMode === 'dark' || (nextMode === 'system' && systemDark.value)
  const applyTheme = async () => {
    mode.value = nextMode
    await nextTick()
  }
  if (!origin || nextIsDark === isDark.value ||
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

export function useTheme() {
  return {
    mode: computed(() => mode.value),
    isDark,
    isTransitioning: computed(() => isTransitioning.value),
    cycle,
  }
}
