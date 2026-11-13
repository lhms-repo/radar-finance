import { useEffect, useRef, useState } from "react";

/** Counts the layers opened, so each step added to the history knows its own. */
let layers = 0;

/** Step of a layer closed by the screen, still waiting to be taken back; a layer opened in the same moment reuses it. */
let pendingBack: ReturnType<typeof setTimeout> | null = null;

/** Tells whether the current step of the history is the one a layer added. */
function isLayer(id: number) {
  return (history.state as { radarLayer?: number } | null)?.radarLayer === id;
}

/**
 * Lets the back button or gesture close an open sheet, dialog or menu before leaving the screen (RNF-26).
 * Opening adds a step to the history; back takes that step and closes the layer, and closing by the screen takes
 * the step back, so no extra back is left behind.
 */
export function useCloseOnBack(open: boolean, close: () => void) {
  const closeRef = useRef(close);
  useEffect(() => {
    closeRef.current = close;
  });
  useEffect(() => {
    if (!open) return;
    const id = ++layers;
    const state = { ...(history.state as object | null), radarLayer: id };
    if (pendingBack !== null) {
      // A layer closed as this one opened, like the Add shortcuts opening the quick entry: take over its step.
      clearTimeout(pendingBack);
      pendingBack = null;
      history.replaceState(state, "");
    } else {
      history.pushState(state, "");
    }
    let wentBack = false;
    const onPop = () => {
      // A layer opened above this one went back to this step; this one stays open.
      if (isLayer(id)) return;
      wentBack = true;
      closeRef.current();
    };
    window.addEventListener("popstate", onPop);
    return () => {
      window.removeEventListener("popstate", onPop);
      // Closed by the screen while its step is still current: take it back, unless a navigation already replaced it.
      if (!wentBack && isLayer(id)) {
        pendingBack = setTimeout(() => {
          pendingBack = null;
          if (isLayer(id)) history.back();
        }, 0);
      }
    };
  }, [open]);
}

/** Open state of a menu that the back button closes (RNF-26). */
export function useBackLayer(initial = false) {
  const [open, setOpen] = useState(initial);
  useCloseOnBack(open, () => setOpen(false));
  return { open, setOpen };
}
