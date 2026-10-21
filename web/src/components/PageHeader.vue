<script setup lang="ts">
defineProps<{
  title: string
  subtitle?: string
}>()
</script>

<template>
  <header class="page-heading">
    <div>
      <h1>{{ title }}</h1>
      <p v-if="subtitle">{{ subtitle }}</p>
    </div>
    <div v-if="$slots.actions" class="m3-actions"><slot name="actions" /></div>
  </header>
</template>

<style scoped>
.page-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 16px;
  margin-bottom: 24px;
}
h1 {
  margin: 0;
  color: var(--md-sys-color-on-surface);
  font-size: 28px;
  font-weight: 400;
  line-height: 36px;
}
p {
  margin: 8px 0 0;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 14px;
  line-height: 20px;
}
.m3-page > .page-heading {
  margin-bottom: 0;
}
</style>
