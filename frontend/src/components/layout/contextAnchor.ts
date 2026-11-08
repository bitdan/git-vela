export function contextAnchor(event: MouseEvent | KeyboardEvent) {
  const trigger = event.currentTarget as HTMLElement
  const pointer = event instanceof MouseEvent && (event.clientX !== 0 || event.clientY !== 0)
  const bounds = trigger.getBoundingClientRect()
  return { trigger, x: pointer ? event.clientX : bounds.left, y: pointer ? event.clientY : bounds.bottom }
}

export type ContextAnchor = ReturnType<typeof contextAnchor>
