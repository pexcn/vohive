import { computed, ref, watch } from 'vue'

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

function cycle() {
  mode.value = modes[(modes.indexOf(mode.value) + 1) % modes.length]!
}

export function useTheme() {
  return { mode: computed(() => mode.value), isDark, cycle }
}
