<script setup lang="ts">
defineProps<{
  rows?: number
}>()
</script>

<template>
  <div class="m3-card list-skeleton" role="status" aria-label="正在加载">
    <span v-for="row in rows || 6" :key="row" aria-hidden="true" />
  </div>
</template>

<style scoped>
.list-skeleton {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.list-skeleton span {
  height: 20px;
  border-radius: 4px;
  background: var(--md-sys-color-surface-container-highest);
}
.list-skeleton span:last-child {
  width: 60%;
}
</style>
