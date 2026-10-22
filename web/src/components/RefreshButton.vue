<script setup lang="ts">
import refresh from '@material-symbols/svg-400/outlined/refresh.svg?raw'
import SvgIcon from './SvgIcon.vue'

const props = defineProps<{
  loading?: boolean
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'click'): void
}>()

function refreshIfIdle() {
  if (!props.disabled && !props.loading) emit('click')
}
</script>

<template>
  <md-outlined-button
    type="button"
    :disabled="disabled"
    :aria-disabled="disabled || loading"
    :aria-busy="!!loading"
    @click="refreshIfIdle"
  >
    <SvgIcon :svg="refresh" slot="icon" class="refresh-icon" :class="{ busy: loading }" />
    刷新
  </md-outlined-button>
</template>

<style scoped>
.refresh-icon.busy {
  animation: refresh-spin 900ms linear 150ms infinite;
}
@keyframes refresh-spin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .refresh-icon.busy {
    animation: none;
  }
}
</style>
