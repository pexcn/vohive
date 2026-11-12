<script setup lang="ts">
import { computed } from 'vue'
import autoMode from '@material-symbols/svg-400/outlined/brightness_auto.svg?raw'
import darkMode from '@material-symbols/svg-400/outlined/dark_mode.svg?raw'
import lightMode from '@material-symbols/svg-400/outlined/light_mode.svg?raw'
import { themeOrigin, useTheme } from '../composables/useTheme'
import SvgIcon from './SvgIcon.vue'

const { mode, isTransitioning, cycle } = useTheme()
const themes = {
  system: { icon: autoMode, label: '跟随设备' },
  light: { icon: lightMode, label: '浅色模式' },
  dark: { icon: darkMode, label: '深色模式' },
}
const current = computed(() => themes[mode.value])

function changeTheme(event: Event) {
  void cycle(themeOrigin(event))
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
