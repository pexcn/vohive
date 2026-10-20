import { reactive } from 'vue'

const LAST_SEEN_PREFIX = 'sms_thread_last_seen:'

function browserStorage(): Storage | undefined {
  try {
    return window.localStorage
  } catch {
    return undefined
  }
}

function timestamp(value: unknown): number {
  const ts = Number(value)
  return Number.isFinite(ts) && ts > 0 ? ts : 0
}

export function useSmsThreadReadState(storage = browserStorage()) {
  const lastSeen = reactive(new Map<string, number>())

  try {
    for (let i = 0; storage && i < storage.length; i++) {
      const key = storage.key(i) || ''
      if (!key.startsWith(LAST_SEEN_PREFIX)) continue
      const suffix = key.slice(LAST_SEEN_PREFIX.length)
      const peerSeparator = suffix.indexOf('|')
      if (peerSeparator < 0) continue
      // Older keys included a device/all filter before the IMSI|peer identity.
      // Merge every legacy view so changing filters cannot revive unread dots.
      const scopeSeparator = suffix.lastIndexOf(':', peerSeparator)
      const threadKey = suffix.slice(scopeSeparator + 1)
      const ts = timestamp(storage.getItem(key))
      lastSeen.set(threadKey, Math.max(lastSeen.get(threadKey) || 0, ts))
    }
  } catch {
    // Restricted browser storage must not prevent reading within this page.
  }

  function getLastSeen(threadKey: string): number {
    const cached = lastSeen.get(threadKey) || 0
    try {
      // Polling renders should also notice reads completed in another tab.
      return Math.max(cached, timestamp(storage?.getItem(LAST_SEEN_PREFIX + threadKey)))
    } catch {
      return cached
    }
  }

  function setLastSeen(threadKey: string, ts: number) {
    const key = LAST_SEEN_PREFIX + threadKey
    const next = Math.max(getLastSeen(threadKey), timestamp(ts))
    try {
      storage?.setItem(key, String(next))
    } catch {
      // Keep the reactive marker even when persistence is unavailable.
    }
    lastSeen.set(threadKey, next)
  }

  return { getLastSeen, setLastSeen }
}
