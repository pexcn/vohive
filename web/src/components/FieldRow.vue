<script setup lang="ts">
import { computed } from 'vue'
import { copyToClipboard } from '../utils/clipboard'
import SensitiveText from './SensitiveText.vue'

const props = defineProps<{
  label: string
  value?: unknown
  copyable?: boolean
  monospace?: boolean
  placeholder?: string
  sensitive?: boolean // 模糊显示敏感信息
}>()

const displayValue = computed(() => {
  const raw = props.value
  const s = raw == null ? '' : String(raw)
  const trimmed = s.trim()
  return trimmed || props.placeholder || '--'
})

const canCopy = computed(() => {
  if (!props.copyable || props.sensitive) return false
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
      :aria-label="canCopy ? `复制${label}：${displayValue}` : undefined"
      class="field-value block min-w-0 max-w-full truncate text-right"
      :class="[
        monospace ? 'font-mono' : '',
        canCopy ? 'cursor-pointer hover:underline' : ''
      ]"
      :title="titleValue"
      @click="copy"
    >
      <SensitiveText :concealed="sensitive"><slot>{{ displayValue }}</slot></SensitiveText>
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
