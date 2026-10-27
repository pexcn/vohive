import { reactive } from 'vue'
import type { VoWiFiRuntimeState } from '../types/api'

type RuntimeDevice = { id: string; vowifi_runtime?: VoWiFiRuntimeState }
type RuntimeSnapshot = { runtime?: VoWiFiRuntimeState; revision: number }

export function useVoWiFiRuntime() {
  const snapshots = reactive(new Map<string, RuntimeSnapshot>())
  let revision = 0

  function checkpoint() {
    return revision
  }

  function update(device: RuntimeDevice, since = revision) {
    const current = snapshots.get(device.id)
    const incomingTime = Date.parse(device.vowifi_runtime?.updated_at || '')
    const currentTime = Date.parse(current?.runtime?.updated_at || '')
    const comparable = Number.isFinite(incomingTime) && Number.isFinite(currentTime)
    if (comparable && incomingTime < currentTime) return
    // A request started before a live update must not overwrite it with an
    // equally old or unversioned snapshot, including an absent runtime.
    if (current && current.revision > since && (!comparable || incomingTime <= currentTime)) return
    snapshots.set(device.id, { runtime: device.vowifi_runtime, revision: ++revision })
  }

  function apply<T extends RuntimeDevice>(device: T): T {
    const snapshot = snapshots.get(device.id)
    return snapshot ? { ...device, vowifi_runtime: snapshot.runtime } : device
  }

  return { checkpoint, update, apply }
}
