<script setup lang="ts">
import { computed } from 'vue'
import { copyToClipboard } from '../utils/clipboard'

const props = defineProps<{
  label: string
  value?: unknown
  copyable?: boolean
  monospace?: boolean
  placeholder?: string
  sensitive?: boolean // 加密显示（打码）
}>()

const displayValue = computed(() => {
  const raw = props.value
  const s = raw == null ? '' : String(raw)
  const trimmed = s.trim()
  return trimmed || props.placeholder || '--'
})

const canCopy = computed(() => {
  if (!props.copyable) return false
  const v = displayValue.value
  return !!v && v !== '--' && v !== '---'
})

const titleValue = computed(() => {
  if (props.sensitive) return ''
  const v = displayValue.value
  return v === '--' || v === '---' ? '' : v
})

async function copy() {
  if (!canCopy.value) return
  await copyToClipboard(displayValue.value, '已复制')
}
</script>

<template>
  <div class="flex w-full min-w-0 items-center justify-between gap-3 overflow-hidden">
    <span class="field-label shrink-0 whitespace-nowrap">{{ label }}</span>
    <component
      :is="canCopy ? 'button' : 'span'"
      :type="canCopy ? 'button' : undefined"
      :aria-label="canCopy ? (sensitive ? `复制${label}` : `复制${label}：${displayValue}`) : undefined"
      class="field-value block min-w-0 max-w-full flex-1 truncate text-right"
      :class="[
        monospace ? 'font-mono' : '',
        canCopy ? 'cursor-pointer hover:underline' : '',
        sensitive ? 'blur-sm select-none motion-safe:transition-all' : ''
      ]"
      :title="titleValue"
      @click="copy"
    >
      <slot>{{ displayValue }}</slot>
    </component>
  </div>
</template>

<style scoped>
.field-value:focus-visible {
  outline: 2px solid var(--md-sys-color-primary);
  outline-offset: -2px;
  border-radius: 4px;
}
.field-label {
  color: var(--md-sys-color-on-surface-variant);
}
</style>
