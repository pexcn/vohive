import test from 'node:test'
import assert from 'node:assert/strict'
import { computed } from 'vue'
import { useVoWiFiRuntime } from '../src/composables/useVoWiFiRuntime'
import { getVoWiFiReadiness, getVoWiFiStatus } from '../src/utils/vowifiStatus'
import type { VoWiFiRuntimeState } from '../src/types/api'

const ready: VoWiFiRuntimeState = {
  sim_ready: true, access_ready: true, tunnel_ready: true, ims_ready: true, sms_ready: true
}
const timestamp = (second: number) => `2026-10-27T00:00:${String(second).padStart(2, '0')}Z`
const device = (second: number, sms = true, id = 'fixture') => ({
  id, vowifi_runtime: { ...ready, sms_ready: sms, updated_at: timestamp(second) }
})

test('Wi-Fi Calling requires every connection stage to be ready', () => {
  assert.equal(getVoWiFiStatus(ready), 'ok')
  for (const field of ['sim_ready', 'access_ready', 'tunnel_ready', 'ims_ready', 'sms_ready']) {
    assert.equal(getVoWiFiStatus({ ...ready, [field]: false }), 'partial', field)
    assert.equal(getVoWiFiStatus({ ...ready, [field]: undefined }), 'partial', field)
  }
  assert.equal(getVoWiFiStatus(undefined), 'off')
  assert.equal(getVoWiFiStatus({}), 'off')
  assert.equal(getVoWiFiStatus({ sim_ready: false, ims_ready: false }), 'off')
  assert.deepEqual(getVoWiFiReadiness(ready).map(item => item.key), ['SIM', 'Access', 'Tunnel', 'IMS', 'SMS'])
})

test('list and detail projections react to the same latest runtime', () => {
  const state = useVoWiFiRuntime()
  const original = device(1, false)
  const list = computed(() => state.apply(original))
  const detail = computed(() => state.apply({ ...original, name: 'Fixture' }))
  state.update(original)
  assert.equal(getVoWiFiStatus(list.value.vowifi_runtime), 'partial')
  state.update(device(2))
  assert.equal(getVoWiFiStatus(list.value.vowifi_runtime), 'ok')
  assert.equal(getVoWiFiStatus(detail.value.vowifi_runtime), 'ok')
  state.update(device(3, false))
  assert.equal(getVoWiFiStatus(list.value.vowifi_runtime), 'partial')
  assert.equal(getVoWiFiStatus(detail.value.vowifi_runtime), 'partial')
  assert.equal(original.vowifi_runtime.updated_at, timestamp(1), 'Input snapshots stay unchanged')
})

test('older HTTP responses and SSE frames cannot roll back the runtime', () => {
  const state = useVoWiFiRuntime()
  state.update(device(1, false))
  const request = state.checkpoint()
  state.update(device(3))
  state.update(device(2, false), request)
  state.update(device(1, false))
  assert.equal(state.apply({ id: 'fixture' }).vowifi_runtime?.updated_at, timestamp(3))
  state.update(device(4, false), request)
  assert.equal(state.apply({ id: 'fixture' }).vowifi_runtime?.updated_at, timestamp(4), 'Newer polling data is accepted immediately')
})

test('late responses without timestamps or runtime cannot erase a live update', () => {
  for (const stale of [{ ...ready, sms_ready: false }, undefined, { ...ready, updated_at: 'invalid' }]) {
    const state = useVoWiFiRuntime()
    const request = state.checkpoint()
    state.update({ id: 'fixture', vowifi_runtime: ready })
    state.update({ id: 'fixture', vowifi_runtime: stale }, request)
    assert.equal(getVoWiFiStatus(state.apply({ id: 'fixture' }).vowifi_runtime), 'ok')
  }
})

test('equal timestamps favor live updates over requests already in flight', () => {
  const state = useVoWiFiRuntime()
  const request = state.checkpoint()
  state.update(device(1))
  state.update(device(1, false), request)
  assert.equal(getVoWiFiStatus(state.apply({ id: 'fixture' }).vowifi_runtime), 'ok')
  state.update(device(1, false))
  assert.equal(getVoWiFiStatus(state.apply({ id: 'fixture' }).vowifi_runtime), 'partial')
})

test('a current runtime reset is reflected immediately and devices remain independent', () => {
  const state = useVoWiFiRuntime()
  state.update(device(1))
  state.update(device(1, false, 'other'))
  state.update({ id: 'fixture' })
  assert.equal(getVoWiFiStatus(state.apply({ id: 'fixture' }).vowifi_runtime), 'off')
  assert.equal(getVoWiFiStatus(state.apply({ id: 'other' }).vowifi_runtime), 'partial')
  const untouched = { id: 'unseen', name: 'Not cached' }
  assert.equal(state.apply(untouched), untouched)
})
