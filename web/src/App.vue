<script setup lang="ts">
import { computed, defineAsyncComponent, provide } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from './stores/auth'
import LoadingScreen from './components/LoadingScreen.vue'
import MaterialFeedback from './components/MaterialFeedback.vue'
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
    <MaterialFeedback />
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
  width: 6px;
  height: 6px;
}
::-webkit-scrollbar-track {
  background: transparent;
}
::-webkit-scrollbar-thumb {
  background: var(--md-sys-color-outline-variant);
  border-radius: 4px;
}
::-webkit-scrollbar-thumb:hover {
  background: var(--md-sys-color-outline);
}
::-webkit-scrollbar-thumb:active {
  background: var(--md-sys-color-on-surface-variant);
}
@supports not selector(::-webkit-scrollbar) {
  * {
    scrollbar-width: thin;
    scrollbar-color: var(--md-sys-color-outline-variant) transparent;
  }
}
@media (max-width: 767px) {
  ::-webkit-scrollbar {
    width: 3px;
    height: 3px;
  }
}
</style>
