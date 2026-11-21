// @ts-check

/**
 * Keep keyboard focus inside an open modal dialog.
 *
 * @param {KeyboardEvent} event
 * @param {HTMLElement} dialog
 */
export function trapDialogFocus(event, dialog) {
  const focusableControls = [.../** @type {NodeListOf<HTMLElement>} */ (dialog.querySelectorAll(
    'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
  ))].filter((control) => control.getClientRects().length > 0);
  const firstControl = focusableControls[0];
  const lastControl = focusableControls[focusableControls.length - 1];
  if (event.shiftKey && document.activeElement === firstControl) {
    event.preventDefault();
    lastControl.focus();
    return;
  }
  if (!event.shiftKey && document.activeElement === lastControl) {
    event.preventDefault();
    firstControl.focus();
  }
}
