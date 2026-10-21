<script setup lang="ts">
defineProps<{
  title: string
  subtitle?: string
}>()
</script>

<template>
  <section class="empty-state">
    <div class="empty-icon" aria-hidden="true">
      <slot name="icon">
        <span class="text-xl font-bold">∅</span>
      </slot>
    </div>
    <h2>{{ title }}</h2>
    <p v-if="subtitle">{{ subtitle }}</p>
    <div v-if="$slots.actions" class="empty-actions">
      <slot name="actions" />
    </div>
  </section>
</template>

<style scoped>
.empty-state {
  padding: 40px 24px;
  text-align: center;
  border-radius: 12px;
  background: var(--md-sys-color-surface-container-low);
  color: var(--md-sys-color-on-surface);
}
.empty-icon {
  display: grid;
  place-items: center;
  width: 48px;
  height: 48px;
  margin: 0 auto 16px;
  border-radius: 16px;
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}
h2 {
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
}
p {
  margin-top: 8px;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 14px;
  line-height: 20px;
}
.empty-actions {
  display: flex;
  justify-content: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 24px;
}
</style>
