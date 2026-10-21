<script setup lang="ts">
import { computed } from 'vue'
import autoMode from '@material-symbols/svg-400/outlined/brightness_auto.svg?raw'
import darkMode from '@material-symbols/svg-400/outlined/dark_mode.svg?raw'
import lightMode from '@material-symbols/svg-400/outlined/light_mode.svg?raw'
import { useTheme } from '../composables/useTheme'
import SvgIcon from './SvgIcon.vue'

const { mode, cycle } = useTheme()
const themes = {
  system: { icon: autoMode, label: '跟随设备' },
  light: { icon: lightMode, label: '亮色模式' },
  dark: { icon: darkMode, label: '暗色模式' },
}
const current = computed(() => themes[mode.value])
</script>

<template>
  <md-icon-button
    :aria-label="`主题：${current.label}`"
    :title="`主题：${current.label}`"
    @click="cycle"
  >
    <SvgIcon :svg="current.icon" />
  </md-icon-button>
</template>
