import type { CardPolicy } from '../types/api'

export type ConnectionMode = 'standby' | 'network' | 'vowifi' | 'airplane'

export function cardConnectionMode(policy: Pick<CardPolicy, 'vowifi_enabled' | 'airplane_enabled' | 'network_enabled'>): ConnectionMode {
  return policy.vowifi_enabled ? 'vowifi' : policy.airplane_enabled ? 'airplane' : policy.network_enabled ? 'network' : 'standby'
}

export const connectionModeLabels: Record<ConnectionMode, string> = {
  standby: '仅驻网', network: '蜂窝数据', vowifi: 'VoWiFi', airplane: '飞行模式'
}

export const policyIPLabels = { v4: 'IPv4', v6: 'IPv6', v4v6: 'IPv4 + IPv6' }
