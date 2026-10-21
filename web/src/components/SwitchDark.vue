<script setup lang="ts">
import darkMode from '@material-symbols/svg-400/outlined/dark_mode.svg?raw'
import lightMode from '@material-symbols/svg-400/outlined/light_mode.svg?raw'
import SvgIcon from './SvgIcon.vue'

const props = defineProps<{
  isDark: boolean
}>()

const emit = defineEmits<{
  toggle: [next: boolean]
}>()

function onToggle() {
  emit('toggle', !props.isDark)
}
</script>

<template>
  <md-icon-button :aria-label="isDark ? '切换为浅色主题' : '切换为深色主题'" @click="onToggle">
    <SvgIcon :svg="isDark ? lightMode : darkMode" />
  </md-icon-button>
</template>
