import type { ObjectDirective } from 'vue'
import type { MdSwitch } from '@material/web/switch/switch.js'

async function synchronize(element: MdSwitch) {
  await element.updateComplete
  // Material Web 2.5 binds the checked attribute. After user input, that
  // attribute no longer updates the live checkbox value on a rollback.
  const input = element.shadowRoot?.querySelector<HTMLInputElement>('input[type="checkbox"]')
  if (input) input.checked = element.selected
}

export const syncMaterialSwitch: ObjectDirective<MdSwitch> = {
  mounted: synchronize,
  updated: synchronize
}
