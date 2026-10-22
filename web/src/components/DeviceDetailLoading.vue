<script setup lang="ts">
import ListSkeleton from './ListSkeleton.vue'
</script>
<template>
  <section class="m3-card m3-stack" role="status" aria-label="正在加载设备详情">
    <h2 class="m3-title">正在读取设备信息…</h2>
    <ListSkeleton :rows="3" />
  </section>
</template>
