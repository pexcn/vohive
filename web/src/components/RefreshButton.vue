<script setup lang="ts">
import refresh from '@material-symbols/svg-400/outlined/refresh.svg?raw'
import SvgIcon from './SvgIcon.vue'

defineProps<{
  loading?: boolean
  disabled?: boolean
}>()

defineEmits<{
  (e: 'click'): void
}>()
</script>

<template>
  <md-outlined-button type="button" :disabled="disabled || loading" :aria-busy="loading" @click="$emit('click')">
    <md-circular-progress v-if="loading" slot="icon" indeterminate aria-hidden="true" />
    <SvgIcon v-else :svg="refresh" slot="icon" class="refresh-icon" />
    刷新
  </md-outlined-button>
</template>

<style scoped>
.refresh-icon {
  width: 18px;
  height: 18px;
}

md-circular-progress {
  --md-circular-progress-size: 18px;
  --md-circular-progress-active-indicator-color: currentColor;
}
</style>
