<script setup lang="ts">
import { themeOrigin, useTheme, type ThemeMode } from '../composables/useTheme'
import { ref, watch } from 'vue'
import { normalizeCustomColor } from '../theme/customColor'
import { fieldValue } from '../utils/material'

const { mode, isDark, colorScheme, colorSchemes, customColor, isTransitioning, setMode, setColorScheme, setCustomColor, resetColors } = useTheme()
const draft = ref(customColor.value)
const error = ref('')
watch([customColor, () => colorScheme.value.id], () => {
  draft.value = customColor.value
  error.value = ''
})

function updateDraft(event: Event) {
  draft.value = fieldValue(event)
  error.value = ''
}

function applyCustom(event: Event) {
  if (!normalizeCustomColor(draft.value)) {
    error.value = '请输入 #RRGGBB 格式的颜色代码，例如 #65568C。'
    return
  }
  error.value = ''
  void setCustomColor(draft.value, themeOrigin(event))
}
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
    <form v-if="colorScheme.id === 'custom'" class="appearance-custom" @submit.prevent>
      <div class="appearance-custom-fields">
        <label class="appearance-picker">
          <span>选取主色</span>
          <input type="color" aria-label="选取自定义主色"
            :value="normalizeCustomColor(draft) || customColor" :disabled="isTransitioning" @input="updateDraft" />
        </label>
        <md-outlined-text-field label="主色代码" :value="draft" :error="!!error" :disabled="isTransitioning"
          @input="updateDraft" @keydown.enter.prevent="applyCustom" />
        <md-filled-button type="button" :disabled="isTransitioning" @click="applyCustom">应用颜色</md-filled-button>
      </div>
      <p v-if="error" class="appearance-error" role="alert">{{ error }}</p>
      <p class="m3-muted">从主色生成完整的深浅配色，点击“应用颜色”后保存。</p>
    </form>
    <div><md-text-button :disabled="isTransitioning" @click="resetColors(themeOrigin($event))">恢复默认配色</md-text-button></div>
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
.appearance-custom { display: grid; gap: 12px; }
.appearance-custom-fields { display: flex; align-items: center; flex-wrap: wrap; gap: 16px; }
.appearance-custom-fields md-outlined-text-field { flex: 1; min-width: 140px; }
.appearance-picker { display: grid; gap: 8px; font-size: 12px; }
.appearance-picker input { width: 48px; height: 48px; padding: 4px; border: 1px solid var(--md-sys-color-outline); border-radius: var(--md-shape-sm); background: var(--md-sys-color-surface-container); cursor: pointer; }
.appearance-error { margin: 0; color: var(--md-sys-color-error); }
@media (max-width: 480px) {
  .appearance-modes, .appearance-colors { gap: 8px; }
  .appearance-modes button { flex: 1; padding: 12px 6px; font-size: 14px; white-space: nowrap; }
  .appearance-color { flex: 1; padding: 12px; gap: 8px; }
}
</style>
