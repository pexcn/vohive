<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps<{ message: string; duration?: number }>()
const emit = defineEmits<{ dismiss: [] }>()
let timer: ReturnType<typeof setTimeout> | undefined
watch(
  () => props.message,
  (message) => {
    clearTimeout(timer)
    if (message && props.duration !== 0)
      timer = setTimeout(() => emit('dismiss'), props.duration ?? 8000)
  },
  { immediate: true }
)
const region = ref<HTMLElement | null>(null)
const messageEl = ref<HTMLElement | null>(null)
const bottom = ref(24)
const availableHeight = ref(240)
const scrollable = ref(false)
let stopLayout = () => {}
onMounted(() => {
  let anchor: Element | null = null
  let frame = 0
  const viewport = window.visualViewport
  const observer = new ResizeObserver(schedule)
  function measure() {
    frame = 0
    const nextAnchor = window.matchMedia('(max-width: 767px)').matches
      ? region.value?.closest('md-dialog')?.querySelector('[slot="actions"]:not(.notice-layer)') ||
        document.querySelector('.sms-composer')
      : null
    if (nextAnchor !== anchor) {
      if (anchor) observer.unobserve(anchor)
      anchor = nextAnchor
      if (anchor) observer.observe(anchor)
    }
    const height = viewport?.height ?? window.innerHeight
    const top = viewport?.offsetTop ?? 0
    const edge = top + height
    const rect = anchor?.getBoundingClientRect()
    // Keep mobile feedback above the composer or modal actions, including when
    // the keyboard changes the visual viewport. Leave room for closing it.
    const gap =
      rect && rect.top < edge && rect.bottom > top
        ? Math.min(Math.max(24, edge - rect.top + 12), Math.max(24, height - 80))
        : 24
    bottom.value = Math.max(0, window.innerHeight - edge) + gap
    availableHeight.value = Math.max(40, height - gap - 40)
    if (messageEl.value)
      scrollable.value = messageEl.value.scrollHeight > messageEl.value.clientHeight
  }
  function schedule() {
    if (!frame) frame = requestAnimationFrame(measure)
  }
  if (messageEl.value) observer.observe(messageEl.value)
  window.addEventListener('resize', schedule)
  viewport?.addEventListener('resize', schedule)
  viewport?.addEventListener('scroll', schedule)
  document.addEventListener('scroll', schedule, true)
  document.addEventListener('opened', schedule, true)
  document.addEventListener('closed', schedule, true)
  document.addEventListener('focusin', schedule)
  measure()
  stopLayout = () => {
    cancelAnimationFrame(frame)
    observer.disconnect()
    window.removeEventListener('resize', schedule)
    viewport?.removeEventListener('resize', schedule)
    viewport?.removeEventListener('scroll', schedule)
    document.removeEventListener('scroll', schedule, true)
    document.removeEventListener('opened', schedule, true)
    document.removeEventListener('closed', schedule, true)
    document.removeEventListener('focusin', schedule)
  }
})
onBeforeUnmount(() => {
  clearTimeout(timer)
  stopLayout()
})
</script>

<template>
  <div
    ref="region"
    class="snackbar-region"
    role="status"
    aria-live="polite"
    aria-atomic="true"
    :style="{
      bottom: `calc(${bottom}px + env(safe-area-inset-bottom, 0px))`,
      '--notice-room': `${availableHeight}px`
    }"
  >
    <div v-if="message" class="snackbar">
      <span
        ref="messageEl"
        class="snackbar-message"
        :tabindex="scrollable ? 0 : undefined"
        :role="scrollable ? 'region' : undefined"
        :aria-label="scrollable ? '通知内容，可滚动查看' : undefined"
        >{{ message }}</span
      >
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
  max-width: min(560px, 100%);
  padding: 4px 8px 4px 16px;
  border-radius: 4px;
  background: var(--md-sys-color-inverse-surface);
  color: var(--md-sys-color-inverse-on-surface);
  box-shadow: var(--md-elevation-3);
  font-size: 14px;
  line-height: 20px;
  pointer-events: auto;
}
.snackbar-message {
  flex: 1;
  min-width: 0;
  max-height: min(240px, var(--notice-room, 40dvh));
  padding: 8px 0;
  overflow: auto;
  overscroll-behavior: contain;
  overflow-wrap: anywhere;
  white-space: pre-line;
}
.snackbar-message:focus-visible {
  outline: 2px solid var(--md-sys-color-inverse-primary);
  outline-offset: -2px;
}
md-text-button {
  flex-shrink: 0;
  --md-text-button-label-text-color: var(--md-sys-color-inverse-primary);
  --md-text-button-hover-label-text-color: var(--md-sys-color-inverse-primary);
  --md-text-button-focus-label-text-color: var(--md-sys-color-inverse-primary);
  --md-text-button-pressed-label-text-color: var(--md-sys-color-inverse-primary);
}
</style>
