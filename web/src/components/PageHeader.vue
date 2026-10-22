<template>
  <header v-if="$slots.actions" class="page-heading" aria-label="页面操作">
    <div class="m3-actions"><slot name="actions" /></div>
  </header>
</template>

<style scoped>
.page-heading {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 16px;
}
</style>
