<script setup lang="ts">
import LoadingScreen from '../components/LoadingScreen.vue'
import SwitchDark from '../components/SwitchDark.vue'
</script>

<template>
  <div class="unauthenticated-shell">
    <div class="absolute top-4 right-4 z-50">
      <SwitchDark />
    </div>
    <router-view v-slot="{ Component }">
      <Suspense>
        <template #default>
          <component :is="Component" />
        </template>
        <template #fallback>
          <LoadingScreen />
        </template>
      </Suspense>
    </router-view>
  </div>
</template>

<style scoped>
.unauthenticated-shell {
  min-height: 100dvh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--md-sys-color-surface);
  color: var(--md-sys-color-on-surface);
}
</style>
