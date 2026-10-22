import { shallowRef } from 'vue'

export type Notice = {
  message: string
  tone?: 'success' | 'info' | 'warning' | 'error'
  duration?: number
}
export const activeNotice = shallowRef<(Notice & { id: number }) | null>(null)
let noticeID = 0

export function notify(notice: Notice) {
  const id = ++noticeID
  activeNotice.value = { ...notice, id }
  return {
    close: () => {
      if (activeNotice.value?.id === id) activeNotice.value = null
    }
  }
}
export function dismissNotices() {
  activeNotice.value = null
}

export type Confirmation = {
  title: string
  message: string
  details?: string
  confirmLabel?: string
  cancelLabel?: string
  danger?: boolean
  alert?: boolean
  input?: { pattern: RegExp; placeholder?: string; errorMessage?: string }
}
type Request = Confirmation & { resolve: (value: string) => void; reject: (reason: string) => void }
export const activeConfirmation = shallowRef<Request | null>(null)
const queued: Request[] = []
let closing = false

export function requestConfirmation(options: Confirmation): Promise<string> {
  return new Promise((resolve, reject) => {
    queued.push({ ...options, resolve, reject })
    showNextConfirmation()
  })
}
export function showNextConfirmation() {
  if (!activeConfirmation.value && !closing) activeConfirmation.value = queued.shift() || null
}
export function finishConfirmation(accepted: boolean, value = '') {
  const request = activeConfirmation.value
  if (!request) return
  closing = true
  activeConfirmation.value = null
  if (accepted || request.alert) request.resolve(value)
  else request.reject('cancel')
}
export function confirmationClosed() {
  closing = false
  showNextConfirmation()
}
