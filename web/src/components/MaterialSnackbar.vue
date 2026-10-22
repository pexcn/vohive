<script setup lang="ts">
import { onBeforeUnmount, watch } from 'vue'

const props = defineProps<{ message: string; duration?: number }>()
const emit = defineEmits<{ dismiss: [] }>()
let timer: ReturnType<typeof setTimeout> | undefined
watch(() => props.message, message => {
  clearTimeout(timer)
  if (message && props.duration !== 0) timer = setTimeout(() => emit('dismiss'), props.duration ?? 8000)
}, { immediate: true })
onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <div class="snackbar-region" role="status" aria-live="polite" aria-atomic="true">
    <div v-if="message" class="snackbar">
      <span>{{ message }}</span>
      <md-text-button type="button" @click="emit('dismiss')">关闭</md-text-button>
    </div>
  </div>
</template>

<style scoped>
.snackbar-region {
  position: fixed;
  inset: auto 16px 24px;
  z-index: 3000;
  display: flex;
  justify-content: center;
  pointer-events: none;
}
.snackbar {
  display: flex;
  align-items: center;
  gap: 16px;
  min-width: min(288px, calc(100vw - 32px));
  max-width: 560px;
  padding: 4px 8px 4px 16px;
  border-radius: 4px;
  background: var(--md-sys-color-inverse-surface);
  color: var(--md-sys-color-inverse-on-surface);
  box-shadow: var(--md-elevation-3);
  font-size: 14px;
  line-height: 20px;
  pointer-events: auto;
}
.snackbar span {
  overflow-wrap: anywhere;
  white-space: pre-line;
}
md-text-button {
  flex-shrink: 0;
  --md-text-button-label-text-color: var(--md-sys-color-inverse-primary);
  --md-text-button-hover-label-text-color: var(--md-sys-color-inverse-primary);
  --md-text-button-focus-label-text-color: var(--md-sys-color-inverse-primary);
  --md-text-button-pressed-label-text-color: var(--md-sys-color-inverse-primary);
}
</style>
