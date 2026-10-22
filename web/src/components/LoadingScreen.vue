<script setup lang="ts">
withDefaults(defineProps<{ title?: string; subtitle?: string }>(), {
  title: '正在加载…',
  subtitle: '正在准备页面'
})
</script>
<template>
  <section class="loading-screen" role="status">
    <md-circular-progress indeterminate aria-label="加载中" />
    <h2 class="m3-title">{{ title }}</h2>
    <p class="m3-muted">{{ subtitle }}</p>
  </section>
</template>
<style scoped>
.loading-screen {
  min-height: 260px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
  padding: 24px;
  background: var(--md-sys-color-surface);
  color: var(--md-sys-color-on-surface);
}
</style>
