<script setup lang="ts">
// SVG content comes only from static build-time imports, never user input.
defineProps<{ svg: string }>()
</script>

<template>
  <span class="svg-icon" aria-hidden="true" v-html="svg" />
</template>

<style scoped>
.svg-icon {
  display: inline-flex;
  width: 24px;
  height: 24px;
}

.svg-icon :deep(svg) {
  width: 100%;
  height: 100%;
  fill: currentColor;
}
</style>
