/** Горячая клавиша встречи действует только вне полей ввода и элементов модального окна. */
export function meetingShortcut(
  event: KeyboardEvent,
  allowed: readonly string[],
  callDialog?: Element | null,
): string | null {
  if (
    event.defaultPrevented ||
    event.repeat ||
    event.isComposing ||
    event.ctrlKey ||
    event.metaKey ||
    event.altKey ||
    event.shiftKey
  )
    return null;
  const target = event.target;
  if (
    callDialog &&
    ((target instanceof Element && !callDialog.contains(target)) ||
      Array.from(document.querySelectorAll('[role="dialog"]')).at(-1) !==
        callDialog)
  )
    return null;
  if (
    target instanceof Element &&
    (target.closest(
      'input, textarea, select, [contenteditable]:not([contenteditable="false"]), [role="textbox"], [role="combobox"]',
    ) ||
      (target.closest('[role="dialog"], [aria-modal="true"]') &&
        target.closest('[role="dialog"], [aria-modal="true"]') !== callDialog))
  )
    return null;
  const key = event.key.toLowerCase();
  return allowed.includes(key) ? key : null;
}
