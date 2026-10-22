<script setup lang="ts">
import { computed, nextTick, onMounted, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import {
  activeNotice,
  activeConfirmation,
  finishConfirmation,
  confirmationClosed
} from '../stores/feedback'
import MaterialSnackbar from './MaterialSnackbar.vue'

const layer = ref<HTMLElement | null>(null)
const confirmationInput = ref('')
const shownConfirmation = shallowRef(activeConfirmation.value)
const validInput = computed(
  () =>
    !shownConfirmation.value?.input ||
    shownConfirmation.value.input.pattern.test(confirmationInput.value)
)
watch(activeConfirmation, (value) => {
  if (value) {
    shownConfirmation.value = value
    confirmationInput.value = ''
  }
})
const noticeTarget = shallowRef<HTMLElement | string>('body')
async function syncNoticeLayer() {
  // A popover outside a modal dialog is inert, even when it is above it.
  // Slot the notice into the current dialog's light DOM so it stays interactive
  // without moving focus or relying on styles crossing a shadow root.
  const dialogs = Array.from(
    document.querySelectorAll<HTMLElement & { open: boolean }>('md-dialog')
  )
  noticeTarget.value =
    dialogs
      .reverse()
      .find((dialog) => dialog.open && dialog.shadowRoot?.querySelector('dialog')?.open) ||
    document.body
  await nextTick()
  const element = layer.value
  if (!element?.showPopover) return
  if (element.matches(':popover-open')) element.hidePopover()
  if (activeNotice.value) element.showPopover()
}
function onDialogTransition(event: Event) {
  if ((event.target as HTMLElement)?.tagName === 'MD-DIALOG') void syncNoticeLayer()
}
watch(activeNotice, syncNoticeLayer, { flush: 'post' })
onMounted(() => {
  document.addEventListener('opened', onDialogTransition, true)
  document.addEventListener('closed', onDialogTransition, true)
  void syncNoticeLayer()
})
onBeforeUnmount(() => {
  document.removeEventListener('opened', onDialogTransition, true)
  document.removeEventListener('closed', onDialogTransition, true)
})
function dismissNotice(id: number) {
  if (activeNotice.value?.id === id) activeNotice.value = null
}
</script>

<template>
  <md-dialog
    :open="!!activeConfirmation"
    @cancel.prevent="finishConfirmation(false)"
    @closed="confirmationClosed"
  >
    <div slot="headline">{{ shownConfirmation?.title }}</div>
    <div slot="content" class="feedback-content">
      <p>{{ shownConfirmation?.message }}</p>
      <md-outlined-text-field
        v-if="shownConfirmation?.input"
        autofocus
        label="确认内容"
        :placeholder="shownConfirmation.input.placeholder"
        :value="confirmationInput"
        :error="!!confirmationInput && !validInput"
        :error-text="shownConfirmation.input.errorMessage"
        @input="confirmationInput = ($event.target as HTMLInputElement).value"
        @keydown.enter.prevent="validInput && finishConfirmation(true, confirmationInput)"
      />
      <pre v-if="shownConfirmation?.details">{{ shownConfirmation.details }}</pre>
    </div>
    <div slot="actions">
      <md-text-button
        v-if="!shownConfirmation?.alert"
        type="button"
        @click="finishConfirmation(false)"
        >{{ shownConfirmation?.cancelLabel || '取消' }}</md-text-button
      >
      <md-filled-button
        type="button"
        :class="{ danger: shownConfirmation?.danger }"
        :disabled="!validInput"
        @click="finishConfirmation(true, confirmationInput)"
        >{{ shownConfirmation?.confirmLabel || '确定' }}</md-filled-button
      >
    </div>
  </md-dialog>
  <Teleport :to="noticeTarget">
    <div ref="layer" slot="actions" popover="manual" class="notice-layer">
      <MaterialSnackbar
        v-if="activeNotice"
        :key="activeNotice.id"
        :message="activeNotice.message"
        :duration="activeNotice.duration ?? (activeNotice.tone === 'error' ? 0 : 8000)"
        @dismiss="activeNotice && dismissNotice(activeNotice.id)"
      />
    </div>
  </Teleport>
</template>

<style scoped>
.notice-layer {
  position: fixed;
  inset: 0;
  width: 100%;
  height: 100%;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  font-family: var(--md-ref-typeface-plain);
  pointer-events: none;
}
.notice-layer::backdrop {
  background: transparent;
}
.feedback-content {
  max-width: min(560px, calc(100vw - 48px));
  padding: 24px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.feedback-content md-outlined-text-field {
  width: 100%;
  margin-top: 16px;
}
.feedback-content pre {
  max-height: 240px;
  overflow: auto;
  margin-top: 16px;
  padding: 12px;
  border-radius: 12px;
  background: var(--md-sys-color-surface-container);
  font: 12px/20px var(--vh-typeface-mono);
  white-space: pre-wrap;
}
md-filled-button.danger {
  --md-filled-button-container-color: var(--md-sys-color-error);
  --md-filled-button-label-text-color: var(--md-sys-color-on-error);
}
</style>
