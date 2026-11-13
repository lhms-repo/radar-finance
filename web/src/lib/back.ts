import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, type NavigateOptions, type To } from "react-router";

/**
 * Back button and gesture that close the open sheet, dialog or menu before leaving the screen (RNF-26).
 *
 * Opening a layer adds a step to the history, marked with an id unique to this page load. Back takes that step and
 * closes the layer; closing by the screen takes the step back. A step whose layer is no longer open, left behind by a
 * menu that opened a dialog or by a reload that kept the old history, is skipped on the way back.
 */

/** Prefix of the ids of this page load, so a step kept from an earlier load is never taken for an open layer. */
const load = Math.random().toString(36).slice(2, 10);

/** Counts the layers opened in this load. */
let layers = 0;

/** Layers open right now. */
const live = new Set<string>();

/** Step of a layer closed by the screen, still waiting to be taken back; a layer opened in the same moment reuses it. */
let pendingBack: ReturnType<typeof setTimeout> | null = null;

/** True while leaveLayers walks back on purpose, so stale steps are not skipped twice. */
let unwinding = false;

/** Returns the layer id of the current step of the history, if any. */
function currentLayer(): string | undefined {
  return (history.state as { radarLayer?: string } | null)?.radarLayer;
}

/** Tells whether the current step of the history is the one a layer added. */
function isLayer(id: string) {
  return currentLayer() === id;
}

if (typeof window !== "undefined") {
  // A reload keeps the history: the step it landed on belongs to no open layer.
  if (currentLayer() !== undefined) {
    const rest = { ...(history.state as Record<string, unknown>) };
    delete rest.radarLayer;
    history.replaceState(rest, "");
  }
  // Skips steps whose layer is gone, so one back press always does something visible.
  window.addEventListener("popstate", () => {
    const id = currentLayer();
    if (!unwinding && id !== undefined && !live.has(id)) history.back();
  });
}

/** Lets the back button or gesture close a layer that is open, before leaving the screen (RNF-26). */
export function useCloseOnBack(open: boolean, close: () => void) {
  const closeRef = useRef(close);
  useEffect(() => {
    closeRef.current = close;
  });
  useEffect(() => {
    if (!open) return;
    const id = `${load}-${++layers}`;
    live.add(id);
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
      live.delete(id);
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

/** Waits for the next step change of the history, or a short while when none comes. */
function nextPop() {
  return new Promise<void>((resolve) => {
    const done = () => {
      window.removeEventListener("popstate", done);
      clearTimeout(timer);
      resolve();
    };
    const timer = setTimeout(done, 500);
    window.addEventListener("popstate", done);
  });
}

/** Walks back over every layer step on top of the screen, so a navigation lands where the screen itself was. */
export async function leaveLayers() {
  if (pendingBack !== null) {
    clearTimeout(pendingBack);
    pendingBack = null;
  }
  unwinding = true;
  try {
    for (let i = 0; i < 10 && currentLayer() !== undefined; i++) {
      history.back();
      await nextPop();
    }
  } finally {
    unwinding = false;
  }
}

/** Navigates from inside a sheet, dialog or menu after leaving its steps, so back never returns to it (RNF-26). */
export function useLeaveNavigate() {
  const navigate = useNavigate();
  return useCallback(
    (to: To, options?: NavigateOptions) => {
      void leaveLayers().then(() => navigate(to, options));
    },
    [navigate],
  );
}
