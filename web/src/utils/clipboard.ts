import { dismissNotices, notify, requestConfirmation } from '../stores/feedback'
export async function copyToClipboard(
  text: unknown,
  successMessage: string = '已复制'
): Promise<boolean> {
  const val = String(text ?? '').trim()
  if (!val) return false

  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(val)
      dismissNotices()
      notify({ tone: 'success', message: successMessage })
      return true
    }
  } catch {
    // Fallback to legacy copy path when clipboard API is blocked.
  }

  try {
    const ta = document.createElement('textarea')
    ta.value = val
    ta.setAttribute('readonly', 'true')
    ta.style.position = 'fixed'
    ta.style.left = '0'
    ta.style.top = '0'
    ta.style.opacity = '0'
    ta.style.pointerEvents = 'none'
    ta.style.width = '1px'
    ta.style.height = '1px'
    document.body.appendChild(ta)
    ta.focus()
    ta.select()
    ta.setSelectionRange(0, ta.value.length)
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    if (ok) {
      dismissNotices()
      notify({ tone: 'success', message: successMessage })
      return true
    }
  } catch {
    // Ignore and fallback to manual prompt.
  }

  dismissNotices()
  notify({ tone: 'warning', message: '浏览器限制，已弹出文本，请手动复制' })
  void requestConfirmation({
    title: '手动复制',
    message: '浏览器未允许自动复制，请选择以下文本后复制。',
    details: val,
    alert: true
  })
  return false
}
