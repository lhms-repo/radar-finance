package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/events"
)

// heartbeat keeps the stream alive through proxies and lets the browser notice a dead server.
const heartbeat = 25 * time.Second

// eventsHandler streams the changes of shared data of the session group as server sent events (RF-02, RF-71).
func eventsHandler(hub *events.Hub, sm *scs.SessionManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Método não permitido.")
			return
		}
		user, group := auth.SessionUser(sm, r)
		if user == 0 || group == 0 {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Entre de novo para continuar.")
			return
		}
		rc := http.NewResponseController(w)
		// The stream stays open far longer than the server write timeout.
		_ = rc.SetWriteDeadline(time.Time{})
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		ch, leave := hub.Subscribe()
		defer leave()
		if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil || rc.Flush() != nil {
			return
		}
		tick := time.NewTicker(heartbeat)
		defer tick.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-tick.C:
				if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
					return
				}
			case e, open := <-ch:
				if !open {
					// The server is shutting down; the browser reconnects to the new one.
					return
				}
				// A change of another group never reaches this screen (RF-02).
				if e.GroupID != group {
					continue
				}
				data, _ := json.Marshal(e)
				if _, err := fmt.Fprintf(w, "event: change\ndata: %s\n\n", data); err != nil || rc.Flush() != nil {
					return
				}
			}
		}
	})
}

// clientHeader carries the id of the browser tab that writes, so only that tab skips its own event.
const clientHeader = "X-Radar-Client"

// maxClientID bounds the tab id echoed to every stream.
const maxClientID = 64

// clientID returns the tab id of the request, or empty when missing or too long.
func clientID(r *http.Request) string {
	id := r.Header.Get(clientHeader)
	if len(id) > maxClientID {
		return ""
	}
	return id
}

// publishChanges announces every successful write of shared data to the open streams.
func publishChanges(hub *events.Hub, sm *scs.SessionManager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if topic := events.Topic(r.URL.Path); topic != "" && rec.status < 400 {
			hub.Publish(events.Event{Topic: topic, GroupID: auth.CurrentGroup(r.Context(), sm), UserID: auth.CurrentUser(r.Context(), sm), ClientID: clientID(r)})
		}
	})
}
