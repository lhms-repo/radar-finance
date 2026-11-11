import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";

/** Wait after the last change before refreshing, so a burst of writes refreshes once. */
const SETTLE_MS = 250;

/** Listens to the server events and refreshes the screens when shared data changes, on this or the other iPhone (RF-71). */
export function useLiveUpdates() {
  const qc = useQueryClient();
  useEffect(() => {
    if (typeof EventSource === "undefined") return;
    const source = new EventSource("/api/events");
    let timer: ReturnType<typeof setTimeout> | undefined;

    // Any change refreshes the queries on screen; the others refresh when they are opened again.
    source.addEventListener("change", () => {
      clearTimeout(timer);
      timer = setTimeout(() => void qc.invalidateQueries(), SETTLE_MS);
    });
    return () => {
      clearTimeout(timer);
      source.close();
    };
  }, [qc]);
}
