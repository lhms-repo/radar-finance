import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { clientId } from "@/api/client";

/** Wait after the last change before refreshing, so a burst of writes refreshes once. */
const SETTLE_MS = 250;

/** First wait before opening the stream again after the browser gave up on it. */
const RETRY_MIN_MS = 2_000;

/** Longest wait between attempts, while the server restarts after a deploy. */
const RETRY_MAX_MS = 30_000;

/** Listens to the server events and refreshes the screens when shared data changes in another tab or device (RF-71); this tab's own changes already refresh where they are made. */
export function useLiveUpdates(meId: number | undefined, groupId: number | undefined) {
  const qc = useQueryClient();
  useEffect(() => {
    // The server sends only the events of the session group, so a new group opens a new stream.
    if (typeof EventSource === "undefined" || meId === undefined || groupId === undefined) return;
    let source: EventSource | undefined;
    let settle: ReturnType<typeof setTimeout> | undefined;
    let retry: ReturnType<typeof setTimeout> | undefined;
    let delay = RETRY_MIN_MS;
    let connectedBefore = false;
    let stopped = false;

    /** Refreshes the queries on screen once the burst of changes settles; the others refresh when opened again. */
    const refresh = () => {
      clearTimeout(settle);
      settle = setTimeout(() => void qc.invalidateQueries(), SETTLE_MS);
    };

    /** Opens the stream; the browser retries network drops itself, but gives up for good on a 502 or 401, so this opens it again. */
    const connect = () => {
      source = new EventSource("/api/events");
      source.addEventListener("open", () => {
        delay = RETRY_MIN_MS;
        // Changes made while the stream was down sent no event, so a reconnection refreshes everything.
        if (connectedBefore) refresh();
        connectedBefore = true;
      });
      source.addEventListener("change", (e) => {
        try {
          const event = JSON.parse((e as MessageEvent<string>).data) as { clientId?: string };
          if (event.clientId === clientId) return;
        } catch {
          // A malformed event still refreshes.
        }
        refresh();
      });
      source.addEventListener("error", () => {
        if (stopped || source?.readyState !== EventSource.CLOSED) return;
        source.close();
        retry = setTimeout(connect, delay);
        delay = Math.min(delay * 2, RETRY_MAX_MS);
      });
    };

    connect();
    return () => {
      stopped = true;
      clearTimeout(settle);
      clearTimeout(retry);
      source?.close();
    };
  }, [qc, meId, groupId]);
}
