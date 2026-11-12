import test from 'node:test'
import assert from 'node:assert/strict'
import { colorSchemes, findColorScheme } from '../src/theme/colorSchemes'
import { appearanceBootstrap } from '../src/theme/bootstrap'
import { runInNewContext } from 'node:vm'

test('the default blue preserves VoHive original light and dark colors', () => {
  const blue = findColorScheme('blue')
  assert.equal(blue.light['--md-sys-color-primary'], '#65568c')
  assert.equal(blue.light['--md-sys-color-surface'], '#f9f6ef')
  assert.equal(blue.dark['--md-sys-color-primary'], '#aac7ff')
  assert.equal(blue.dark['--md-sys-color-surface'], '#111318')
  for (const id of [null, undefined, 'unknown', {}, 'custom']) assert.equal(findColorScheme(id).id, 'blue')
  assert.equal(findColorScheme('green').light['--md-sys-color-primary'], '#007a50')
  assert.equal(findColorScheme('green').dark['--md-sys-color-primary'], '#55dfa5')
})

export function assertReadable(scheme: typeof colorSchemes[number]) {
  function luminance(hex: string) {
    const rgb = hex.slice(1).match(/../g)!.map(value => {
      const channel = parseInt(value, 16) / 255
      return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
    })
    return rgb.reduce((sum, value, index) => sum + value * [0.2126, 0.7152, 0.0722][index], 0)
  }
  for (const mode of ['light', 'dark'] as const) {
    const palette = scheme[mode]
    assert.deepEqual(Object.keys(palette).sort(), Object.keys(colorSchemes[0].light).sort())
    for (const value of Object.values(palette)) assert.match(value, /^#[0-9a-f]{6}$/i)
    for (const [text, background] of [
      ['on-primary', 'primary'], ['on-primary-container', 'primary-container'],
      ['on-secondary', 'secondary'], ['on-secondary-container', 'secondary-container'],
      ['on-tertiary', 'tertiary'], ['on-tertiary-container', 'tertiary-container'],
      ['on-surface', 'surface'], ['on-surface-variant', 'surface-container'],
      ['on-error', 'error'], ['on-error-container', 'error-container']
    ]) {
      const values = [text, background].map(role => luminance(palette[`--md-sys-color-${role}` as keyof typeof palette])).sort((a, b) => b - a)
      assert.ok((values[0] + 0.05) / (values[1] + 0.05) >= 4.5, `${scheme.id}/${mode}: ${text} on ${background}`)
    }
  }
}

test('both presets provide complete palettes with readable text', () => {
  assert.equal(new Set(colorSchemes.map(scheme => scheme.id)).size, colorSchemes.length)
  for (const scheme of colorSchemes) assertReadable(scheme)
})

test('the inline bootstrap restores colors before application modules, including blocked storage', () => {
  for (const color of ['blue', 'green', 'unknown']) {
    for (const mode of ['light', 'dark', 'system', 'unknown', 'blocked']) {
      const styles: Record<string, string> = {}
      const root = { classList: { toggle: (_: string, dark: boolean) => { styles.dark = String(dark) } }, dataset: {}, style: { setProperty: (key: string, value: string) => { styles[key] = value } } }
      runInNewContext(appearanceBootstrap(), {
        document: { documentElement: root, querySelector: () => null },
        localStorage: { getItem: (key: string) => {
          if (mode === 'blocked') throw new Error('Storage blocked')
          return key === 'theme' ? mode : color
        } },
        matchMedia: () => ({ matches: true })
      })
      const expected = findColorScheme(mode === 'blocked' ? null : color)
      assert.equal(styles.dark, String(mode !== 'light'))
      assert.equal(styles['--md-sys-color-primary'], (mode === 'light' ? expected.light : expected.dark)['--md-sys-color-primary'])
    }
  }
})
