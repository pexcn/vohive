const duration = 650
const easing = 'cubic-bezier(0.4, 0, 0.2, 1)'
// Finite endpoints keep the projection stable.
const scales = [0.01, 0.999999]

export async function revealTheme(origin: { x: number; y: number }, update: () => Promise<void>) {
  const root = document.documentElement
  const previousName = root.style.getPropertyValue('view-transition-name')
  const previousPriority = root.style.getPropertyPriority('view-transition-name')
  const coordinates = document.createElement('style')
  coordinates.dataset.themeReveal = ''
  const animations: Animation[] = []
  let transition: ViewTransition | undefined

  function animate(frames: Keyframe[], pseudoElement: string) {
    const animation = root.animate(frames, { duration, easing, fill: 'forwards', pseudoElement })
    animations.push(animation)
    // Observe cancellation even if starting the other animation throws.
    void animation.finished.catch(() => undefined)
    return animation.finished
  }

  root.classList.add('theme-transition')
  try {
    transition = document.startViewTransition(async () => {
      // Separate the old, stationary snapshot from the new circular overlay.
      root.style.setProperty('view-transition-name', 'theme-reveal')
      await update()
    })
    await transition.ready

    // Mobile browser chrome can make the captured viewport larger than
    // innerHeight. Read the actual capture after it exists, including any
    // viewport change during capture, so the last corner is fully covered.
    const capture = getComputedStyle(root, '::view-transition-group(theme-reveal)')
    const overlay = getComputedStyle(root, '::view-transition')
    const width = Math.max(window.innerWidth, root.clientWidth,
      Number.parseFloat(capture.width) || 0, Number.parseFloat(overlay.width) || 0)
    const height = Math.max(window.innerHeight, root.clientHeight,
      Number.parseFloat(capture.height) || 0, Number.parseFloat(overlay.height) || 0)
    const x = Math.min(Math.max(origin.x, 0), width)
    const y = Math.min(Math.max(origin.y, 0), height)
    const radius = Math.hypot(Math.max(x, width - x), Math.max(y, height - y)) + 1

    // Coordinates belong only to the overlay; the clip itself stays constant.
    coordinates.textContent = `
      :root.theme-transition::view-transition-group(theme-reveal),
      :root.theme-transition::view-transition-image-pair(theme-reveal) {
        transform-origin: ${x}px ${y}px;
      }
      :root.theme-transition::view-transition-group(theme-reveal) {
        clip-path: circle(${radius}px at ${x}px ${y}px);
      }
    `
    document.head.append(coordinates)
    // Perspective interpolates its reciprocal distance. After translateZ(1px),
    // its projection scales by 1/s, exactly cancelling the outer scale s.
    await Promise.all([
      animate(scales.map(scale => ({ scale })), '::view-transition-group(theme-reveal)'),
      animate(scales.map(scale => ({
        transform: `perspective(${1 / (1 - scale)}px) translateZ(1px)`,
      })), '::view-transition-image-pair(theme-reveal)'),
    ])
  } catch {
    // A skipped or unsupported transition must still apply the target theme.
    transition?.skipTransition()
    await update()
  } finally {
    if (transition) await transition.finished.catch(() => undefined)
    for (const animation of animations) animation.cancel()
    if (previousName) root.style.setProperty('view-transition-name', previousName, previousPriority)
    else root.style.removeProperty('view-transition-name')
    coordinates.remove()
    root.classList.remove('theme-transition')
  }
}
