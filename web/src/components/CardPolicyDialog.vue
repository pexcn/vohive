<script setup lang="ts">
import { ref, shallowRef, useId, watch } from 'vue'
import type { CardPolicy } from '../types/api'
import CardPolicyPanel from './CardPolicyPanel.vue'
import SvgIcon from './SvgIcon.vue'
import closeIcon from '@material-symbols/svg-400/outlined/close.svg?raw'

const props = defineProps<{
  card: { deviceId: string; iccid: string; name: string } | null
  isActiveCard: boolean
  deviceOnline: boolean
  esimSwitching?: boolean
  concealIccid?: boolean
}>()
const emit = defineEmits<{ close: []; policyChanged: [policy: CardPolicy]; savingChanged: [saving: boolean] }>()
const id = useId()
const open = ref(false)
const saving = ref(false)
const shownCard = shallowRef(props.card)
watch(() => props.card, card => {
  if (card) shownCard.value = card
  open.value = !!card
}, { immediate: true })

function close() {
  if (!saving.value) open.value = false
}
function closed() {
  if (open.value) return
  shownCard.value = null
  emit('close')
}
function onSavingChanged(value: boolean) {
  saving.value = value
  emit('savingChanged', value)
}
function onPolicyChanged(policy: CardPolicy, applyError: string) {
  emit('policyChanged', policy)
  if (!applyError) close()
}
</script>

<template>
  <md-dialog class="m3-dialog card-policy-dialog" :open="open" @cancel.prevent="close" @closed="closed">
    <div slot="headline" class="card-policy-heading">
      <div><span>SIM 卡设置</span><span class="m3-badge">跟随 SIM 卡保存</span></div>
      <md-icon-button aria-label="关闭 SIM 卡设置" :disabled="saving" @click="close"><SvgIcon :svg="closeIcon" /></md-icon-button>
    </div>
    <div slot="content">
      <CardPolicyPanel
        v-if="shownCard"
        :key="`${shownCard.deviceId}:${shownCard.iccid}`"
        :device-id="shownCard.deviceId"
        :iccid="shownCard.iccid"
        :card-name="shownCard.name"
        :device-online="deviceOnline"
        :is-active-card="isActiveCard"
        :esim-switching="esimSwitching"
        :conceal-iccid="concealIccid"
        :actions-target="`#${id}-actions`"
        embedded
        @close="close"
        @saving-changed="onSavingChanged"
        @policy-changed="onPolicyChanged"
      />
    </div>
    <div :id="`${id}-actions`" slot="actions" class="card-policy-actions"></div>
  </md-dialog>
</template>

<style scoped>
.card-policy-dialog {
  width: min(800px, calc(100vw - 48px));
  max-width: calc(100vw - 48px);
  max-height: calc(100dvh - 48px);
}
.card-policy-heading, .card-policy-heading > div {
  display: flex;
  align-items: center;
  gap: 12px;
}
.card-policy-heading { justify-content: space-between; align-items: flex-start; }
.card-policy-heading > div { flex-wrap: wrap; }
.card-policy-heading md-icon-button { flex-shrink: 0; }
.card-policy-dialog [slot='content'] { padding-top: 8px; }
.card-policy-actions { width: 100%; }
@media (max-width: 640px) {
  .card-policy-dialog { width: calc(100vw - 32px); max-width: calc(100vw - 32px); }
}
</style>
