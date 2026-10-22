<script setup lang="ts">
import { computed, defineAsyncComponent, provide } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from './stores/auth'
import LoadingScreen from './components/LoadingScreen.vue'
import { useTheme } from './composables/useTheme'

const route = useRoute()
const auth = useAuthStore()

const { isDark } = useTheme()
provide('isDark', isDark)

const AuthenticatedShell = defineAsyncComponent(() => import('./layouts/AuthenticatedShell.vue'))
const UnauthenticatedShell = defineAsyncComponent(() => import('./layouts/UnauthenticatedShell.vue'))
const shell = computed(() =>
  auth.isAuthenticated && route.name !== 'Login' ? AuthenticatedShell : UnauthenticatedShell
)
</script>

<template>
  <div class="app-root">
    <Suspense>
      <template #default>
        <component :is="shell" />
      </template>
      <template #fallback>
        <LoadingScreen />
      </template>
    </Suspense>
  </div>
</template>

<style>
.app-root {
  position: relative;
  height: 100dvh;
  width: 100%;
  overflow: hidden;
  background: var(--md-sys-color-surface);
  color: var(--md-sys-color-on-surface);
  font-family: var(--md-ref-typeface-plain);
}

/* Custom Scrollbar */
::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}
::-webkit-scrollbar-track {
  background: transparent;
}
::-webkit-scrollbar-thumb {
  background: #cbd5e1;
  border-radius: 4px;
}
.dark ::-webkit-scrollbar-thumb {
  background: #334155;
}
::-webkit-scrollbar-thumb:hover {
  background: #94a3b8;
}
.dark ::-webkit-scrollbar-thumb:hover {
  background: #475569;
}
@media (max-width: 767px) {
  ::-webkit-scrollbar {
    width: 3px;
    height: 3px;
  }
  ::-webkit-scrollbar-thumb,
  .dark ::-webkit-scrollbar-thumb {
    background: color-mix(in srgb, var(--md-sys-color-on-surface) 24%, transparent);
  }
}
</style>
