import '@fontsource/roboto/latin-400.css'
import '@fontsource/roboto/latin-500.css'
import '@fontsource/roboto-mono/latin-400.css'
import '@fontsource/roboto-mono/latin-500.css'
import '@material/web/button/filled-button.js'
import '@material/web/button/filled-tonal-button.js'
import '@material/web/button/outlined-button.js'
import '@material/web/button/text-button.js'
import { MdDialog } from '@material/web/dialog/dialog.js'
import scrollbarStyles from './scrollbars.css?inline'
import '@material/web/iconbutton/icon-button.js'
import { MdMenu } from '@material/web/menu/menu.js'
import '@material/web/menu/menu-item.js'
import '@material/web/progress/circular-progress.js'
import '@material/web/progress/linear-progress.js'
import '@material/web/ripple/ripple.js'
import '@material/web/select/outlined-select.js'
import '@material/web/select/select-option.js'
import '@material/web/switch/switch.js'
import '@material/web/tabs/tabs.js'
import '@material/web/tabs/primary-tab.js'
import { MdOutlinedTextField } from '@material/web/textfield/outlined-text-field.js'

// Reuse the global scrollbar rules inside Material's scrolling Shadow DOMs.
// MdMenu also covers the menus created internally by select components.
const globalScrollbarStyles = new CSSStyleSheet()
globalScrollbarStyles.replaceSync(scrollbarStyles)
for (const component of [MdDialog, MdMenu, MdOutlinedTextField]) {
  component.elementStyles = [...component.elementStyles, globalScrollbarStyles]
}
