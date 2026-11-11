import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";

/** Wait after the last change before refreshing, so a burst of writes refreshes once. */
const SETTLE_MS = 250;

/** Listens to the server events and refreshes the screens when the other person changes shared data (RF-71); own changes already refresh where they are made. */
export function useLiveUpdates(meId: number | undefined) {
  const qc = useQueryClient();
  useEffect(() => {
    if (typeof EventSource === "undefined") return;
    const source = new EventSource("/api/events");
    let timer: ReturnType<typeof setTimeout> | undefined;

    // A change of the other person refreshes the queries on screen; the others refresh when opened again.
    source.addEventListener("change", (e) => {
      try {
        const { userId } = JSON.parse((e as MessageEvent<string>).data) as { userId?: number };
        if (meId !== undefined && userId === meId) return;
      } catch {
        // A malformed event still refreshes.
      }
      clearTimeout(timer);
      timer = setTimeout(() => void qc.invalidateQueries(), SETTLE_MS);
    });
    return () => {
      clearTimeout(timer);
      source.close();
    };
  }, [qc, meId]);
}
