export function fieldValue(event: Event): string {
  return (event.target as HTMLInputElement).value
}
export function fieldSelected(event: Event): boolean {
  return (event.target as HTMLElement & { selected: boolean }).selected
}
export function guardDialogCancel(event: Event, busy: boolean) {
  if (busy) event.preventDefault()
}
