<script setup lang="ts">
import { themeOrigin, useTheme, type ThemeMode } from '../composables/useTheme'

const { mode, isDark, colorScheme, colorSchemes, isTransitioning, setMode, setColorScheme } = useTheme()
const modes: { id: ThemeMode; label: string }[] = [
  { id: 'system', label: '跟随设备' },
  { id: 'light', label: '浅色模式' },
  { id: 'dark', label: '深色模式' }
]
</script>

<template>
  <section class="m3-card m3-stack appearance-settings" aria-label="外观设置">
    <div>
      <h2 class="m3-title">外观</h2>
      <p class="m3-muted">即时生效，自动保存在本浏览器。</p>
    </div>
    <fieldset>
      <legend>明暗模式</legend>
      <div class="appearance-modes">
        <button
          v-for="option in modes" :key="option.id" type="button"
          :aria-pressed="mode === option.id" :disabled="isTransitioning"
          @click="setMode(option.id, themeOrigin($event))"
        ><md-ripple />{{ option.label }}</button>
      </div>
    </fieldset>
    <fieldset>
      <legend>主题颜色</legend>
      <div class="appearance-colors">
        <label v-for="scheme in colorSchemes" :key="scheme.id" class="appearance-color">
          <input
            type="radio" name="appearance-color" :value="scheme.id"
            :checked="colorScheme.id === scheme.id" :disabled="isTransitioning"
            @change="setColorScheme(scheme.id, themeOrigin($event))"
          />
          <span class="appearance-swatch" aria-hidden="true"
            :style="{ background: (isDark ? scheme.dark : scheme.light)['--md-sys-color-primary'] }" />
          <span class="appearance-name">{{ scheme.label }}<small v-if="scheme.id === 'blue'" class="m3-muted">默认</small></span>
        </label>
      </div>
    </fieldset>
  </section>
</template>

<style scoped>
fieldset { min-width: 0; margin: 0; padding: 0; border: 0; }
legend { margin-bottom: 12px; font-size: 14px; font-weight: 500; }
.appearance-modes, .appearance-colors { display: flex; flex-wrap: wrap; gap: 12px; }
.appearance-modes button, .appearance-color {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  min-height: 48px;
  padding: 12px 20px;
  border: 1px solid var(--md-sys-color-outline);
  border-radius: var(--md-shape-md);
  background: var(--md-sys-color-surface-container-low);
  color: var(--md-sys-color-on-surface);
  font: inherit;
  cursor: pointer;
}
.appearance-modes button[aria-pressed="true"], .appearance-color:has(input:checked) {
  border-color: var(--md-sys-color-primary);
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
  box-shadow: inset 0 0 0 1px var(--md-sys-color-primary);
}
.appearance-modes button:focus-visible, .appearance-color:has(input:focus-visible) {
  outline: 3px solid var(--md-sys-color-primary);
  outline-offset: 3px;
}
.appearance-modes button:disabled, .appearance-color:has(input:disabled) { cursor: default; }
.appearance-color input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: inherit; }
.appearance-swatch { width: 28px; height: 28px; flex: none; border-radius: 50%; }
.appearance-name { white-space: nowrap; }
.appearance-name small { display: block; font-size: 12px; }
@media (max-width: 480px) {
  .appearance-modes, .appearance-colors { gap: 8px; }
  .appearance-modes button { flex: 1; padding: 12px 6px; font-size: 14px; white-space: nowrap; }
  .appearance-color { flex: 1; padding: 12px; gap: 8px; }
}
</style>
