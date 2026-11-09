export function contextAnchor(event: MouseEvent | KeyboardEvent, atPointer = true) {
  const trigger = event.currentTarget as HTMLElement
  const pointer = atPointer && event instanceof MouseEvent && (event.clientX !== 0 || event.clientY !== 0)
  const bounds = trigger.getBoundingClientRect()
  return { trigger, toggle: event.type === 'click', x: pointer ? event.clientX : bounds.left, y: pointer ? event.clientY : bounds.bottom }
}

export type ContextAnchor = ReturnType<typeof contextAnchor>
