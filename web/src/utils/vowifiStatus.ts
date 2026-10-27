import type { VoWiFiRuntimeState } from '../types/api'

export function getVoWiFiReadiness(runtime?: VoWiFiRuntimeState) {
  return [
    { key: 'SIM', ready: runtime?.sim_ready },
    { key: 'Access', ready: runtime?.access_ready },
    { key: 'Tunnel', ready: runtime?.tunnel_ready },
    { key: 'IMS', ready: runtime?.ims_ready },
    { key: 'SMS', ready: runtime?.sms_ready }
  ]
}

export function getVoWiFiStatus(runtime?: VoWiFiRuntimeState): 'ok' | 'partial' | 'off' {
  const readiness = getVoWiFiReadiness(runtime)
  if (readiness.every(item => item.ready === true)) return 'ok'
  return readiness.some(item => item.ready === true) ? 'partial' : 'off'
}
