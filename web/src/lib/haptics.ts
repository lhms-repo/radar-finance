/**
 * Optional haptic feedback on iPhone (RNF-25).
 *
 * Safari on iOS 18 vibrates when a native switch input is toggled. Clicking the
 * label of a hidden switch triggers that tick. Nothing in the app depends on it:
 * on other platforms the click simply does nothing.
 */

let label: HTMLLabelElement | null = null;
let enabled = true;

/** Turns haptic feedback on or off for the whole app. */
export function setHapticsEnabled(value: boolean) {
  enabled = value;
}

/** Creates the hidden switch the first time it is needed. */
function ensureSwitch(): HTMLLabelElement {
  if (label) return label;
  label = document.createElement("label");
  label.setAttribute("aria-hidden", "true");
  label.style.cssText =
    "position:fixed;left:-9999px;top:0;width:1px;height:1px;overflow:hidden;opacity:0;pointer-events:none";
  const input = document.createElement("input");
  input.type = "checkbox";
  input.setAttribute("switch", "");
  input.tabIndex = -1;
  label.appendChild(input);
  document.body.appendChild(label);
  return label;
}

/** Plays a light haptic tick when enabled and supported. Call it inside a user gesture. */
export function haptic() {
  if (!enabled || typeof document === "undefined") return;
  try {
    ensureSwitch().click();
  } catch {
    /* Haptics are a bonus; failures are ignored. */
  }
}
