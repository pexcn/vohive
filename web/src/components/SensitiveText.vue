<script setup lang="ts">
defineProps<{ concealed?: boolean }>()
</script>

<template>
  <span class="sensitive-text" :data-concealed="concealed || undefined">
    <span class="sensitive-text-content"><slot /></span>
  </span>
</template>

<style scoped>
.sensitive-text {
  display: inline-block;
  min-width: 0;
  max-width: 100%;
  overflow: hidden;
  vertical-align: bottom;
  text-decoration: inherit;
}
.sensitive-text-content {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  text-decoration: inherit;
}
.sensitive-text[data-concealed] {
  border-radius: 4px;
  user-select: none;
}
.sensitive-text[data-concealed] .sensitive-text-content {
  filter: blur(4px);
}
@media (prefers-reduced-motion: no-preference) {
  .sensitive-text-content {
    transition: filter 150ms cubic-bezier(0.4, 0, 0.2, 1);
  }
}
</style>
