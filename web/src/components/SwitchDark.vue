<script setup lang="ts">
import { computed } from 'vue'
import autoMode from '@material-symbols/svg-400/outlined/brightness_auto.svg?raw'
import darkMode from '@material-symbols/svg-400/outlined/dark_mode.svg?raw'
import lightMode from '@material-symbols/svg-400/outlined/light_mode.svg?raw'
import { useTheme } from '../composables/useTheme'
import SvgIcon from './SvgIcon.vue'

const { mode, isTransitioning, cycle } = useTheme()
const themes = {
  system: { icon: autoMode, label: '跟随设备' },
  light: { icon: lightMode, label: '浅色模式' },
  dark: { icon: darkMode, label: '深色模式' },
}
const current = computed(() => themes[mode.value])

function changeTheme(event: MouseEvent) {
  let { clientX: x, clientY: y } = event
  if (event.detail === 0) {
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
    x = rect.left + rect.width / 2
    y = rect.top + rect.height / 2
  }
  void cycle({ x, y })
}
</script>

<template>
  <md-icon-button
    :aria-label="current.label"
    :title="current.label"
    :aria-busy="isTransitioning || undefined"
    @click="changeTheme"
  >
    <SvgIcon :svg="current.icon" />
  </md-icon-button>
</template>
