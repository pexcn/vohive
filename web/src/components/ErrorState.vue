<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  title?: string
  message: string
  details?: string
  statusCode?: number
  requestMethod?: string
  requestUrl?: string
  lastSuccessAt?: number | null
  retryText?: string
}>()
const emit = defineEmits<{
  (e: 'retry'): void
}>()

const metaText = computed(() => {
  const parts: string[] = []
  if (props.statusCode) parts.push(`HTTP ${props.statusCode}`)
  const method = (props.requestMethod || '').toUpperCase()
  if (method && props.requestUrl) parts.push(`${method} ${props.requestUrl}`)
  else if (props.requestUrl) parts.push(String(props.requestUrl))
  if (props.lastSuccessAt) parts.push(`最后成功：${new Date(props.lastSuccessAt).toLocaleString()}`)
  return parts.join(' · ')
})
</script>

<template>
  <section class="error-state" role="alert">
    <div class="error-content">
      <div class="min-w-0">
        <h2>{{ title || '加载失败' }}</h2>
        <p>{{ message }}</p>
        <div v-if="metaText" class="error-meta">
          {{ metaText }}
        </div>
        <div v-if="details" class="error-meta">{{ details }}</div>
      </div>
      <md-outlined-button v-if="retryText" type="button" @click="emit('retry')">
        {{ retryText }}
      </md-outlined-button>
    </div>
  </section>
</template>

<style scoped>
.error-state {
  padding: 16px;
  border-radius: 12px;
  background: var(--md-sys-color-error-container);
  color: var(--md-sys-color-on-error-container);
}
.error-content {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 16px;
}
h2 {
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
}
p {
  margin-top: 4px;
  font-size: 14px;
  overflow-wrap: anywhere;
}
.error-meta {
  margin-top: 8px;
  font: 12px/16px var(--vh-typeface-mono);
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}
</style>
