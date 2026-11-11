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
    const topics = new Set<string>();
    let timer: ReturnType<typeof setTimeout> | undefined;

    /** Refreshes the market queries for market changes and everything else for other areas. */
    const flush = () => {
      if (topics.size === 1 && topics.has("market")) void qc.invalidateQueries({ queryKey: ["market"] });
      else void qc.invalidateQueries();
      topics.clear();
    };

    source.addEventListener("change", (e) => {
      try {
        const { topic } = JSON.parse((e as MessageEvent<string>).data) as { topic: string };
        topics.add(topic);
      } catch {
        topics.add("all");
      }
      clearTimeout(timer);
      timer = setTimeout(flush, SETTLE_MS);
    });
    return () => {
      clearTimeout(timer);
      source.close();
    };
  }, [qc]);
}
