import test from 'node:test'
import assert from 'node:assert/strict'
import { computed } from 'vue'
import { useSmsThreadReadState } from '../src/composables/useSmsThreadReadState'

function memoryStorage(initial: Record<string, string> = {}): Storage {
  const values = new Map(Object.entries(initial))
  return {
    get length() { return values.size },
    key: index => Array.from(values.keys())[index] ?? null,
    getItem: key => values.get(key) ?? null,
    setItem: (key, value) => { values.set(key, value) },
    removeItem: key => { values.delete(key) },
    clear: () => { values.clear() }
  }
}

test('read state survives reopening under another device filter and remains SIM-specific', () => {
  const storage = memoryStorage()
  const deviceView = useSmsThreadReadState(storage)
  deviceView.setLastSeen('sim-a|sender', 100)
  const allDevicesView = useSmsThreadReadState(storage)
  assert.equal(allDevicesView.getLastSeen('sim-a|sender'), 100)
  assert.equal(allDevicesView.getLastSeen('sim-b|sender'), 0)
  assert.equal(allDevicesView.getLastSeen('sim-a|another-sender'), 0)
})

test('legacy read markers from all filters are merged without marking newer messages read', () => {
  const state = useSmsThreadReadState(memoryStorage({
    'sms_thread_last_seen:device-a:sim-a|sender': '100',
    'sms_thread_last_seen:all:sim-a|sender': '200',
    'sms_thread_last_seen:device-b:sim-b|sender': '300',
    'sms_thread_last_seen:device:with:colons:sim-a|peer:with:colons': '150',
    'sms_thread_last_seen:all:sim-a|invalid': 'Infinity'
  }))
  assert.equal(state.getLastSeen('sim-a|sender'), 200)
  assert.equal(state.getLastSeen('sim-b|sender'), 300)
  assert.equal(state.getLastSeen('sim-a|peer:with:colons'), 150)
  assert.equal(state.getLastSeen('sim-a|invalid'), 0)
  assert.equal(201 > state.getLastSeen('sim-a|sender'), true)
})

test('marking read clears the reactive unread dot immediately and never rolls back', () => {
  const storage = memoryStorage()
  const state = useSmsThreadReadState(storage)
  const unread = computed(() => 200 > state.getLastSeen('sim-a|sender'))
  assert.equal(unread.value, true)
  state.setLastSeen('sim-a|sender', 200)
  assert.equal(unread.value, false)
  state.setLastSeen('sim-a|sender', 100)
  assert.equal(unread.value, false)
  assert.equal(useSmsThreadReadState(storage).getLastSeen('sim-a|sender'), 200)
})

test('an older view cannot overwrite a newer persisted read marker from another tab', () => {
  const storage = memoryStorage()
  const olderTab = useSmsThreadReadState(storage)
  const newerTab = useSmsThreadReadState(storage)
  newerTab.setLastSeen('sim-a|sender', 200)
  assert.equal(olderTab.getLastSeen('sim-a|sender'), 200)
  olderTab.setLastSeen('sim-a|sender', 100)
  assert.equal(useSmsThreadReadState(storage).getLastSeen('sim-a|sender'), 200)
})

test('restricted storage keeps read state reactive within the current page', () => {
  const storage = memoryStorage()
  storage.setItem = () => { throw new Error('storage unavailable') }
  const state = useSmsThreadReadState(storage)
  const unread = computed(() => 100 > state.getLastSeen('sim-a|sender'))
  assert.equal(unread.value, true)
  state.setLastSeen('sim-a|sender', 100)
  assert.equal(unread.value, false)
})
