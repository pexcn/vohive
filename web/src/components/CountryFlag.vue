<script setup lang="ts">
import { computed, ref, watch } from 'vue'

const props = defineProps<{ iso?: string }>()

const flagSources = import.meta.glob<string>(
  '/src/assets/flags/*.svg',
  { query: '?url', import: 'default' }
)
const countryCode = computed(() => props.iso?.trim().toLowerCase() || '')
const source = ref('')

watch(countryCode, async (code, _, onCleanup) => {
  source.value = ''
  let active = true
  onCleanup(() => { active = false })
  if (!/^[a-z]{2}$/.test(code)) return
  const load = flagSources[`/src/assets/flags/${code}.svg`]
  if (!load) return
  try {
    const url = await load()
    if (active) source.value = url
  } catch {
    // Keep the operator text visible if the optional flag cannot load.
  }
}, { immediate: true })
</script>

<template>
  <img
    v-if="source"
    class="country-flag"
    :src="source"
    :alt="`${countryCode.toUpperCase()} 国旗`"
    width="20"
    height="20"
    @error="source = ''"
  >
</template>

<style scoped>
.country-flag {
  display: inline-block;
  width: 20px;
  height: 20px;
  margin-inline-end: 6px;
  vertical-align: -4px;
  object-fit: contain;
}
</style>
