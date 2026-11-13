package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/events"
)

// heartbeat keeps the stream alive through proxies and lets the browser notice a dead server.
const heartbeat = 25 * time.Second

// eventsHandler streams the changes of shared data of the session group as server sent events (RF-02, RF-71).
func eventsHandler(hub *events.Hub, sm *scs.SessionManager, accounts *account.Service) http.Handler {
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
		// The stream skips the session middleware, so it checks the membership itself (RF-02).
		if _, ok, err := accounts.Membership(r.Context(), group, user); err != nil || !ok {
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
func publishChanges(hub *events.Hub, sm *scs.SessionManager, accounts *account.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if topic := events.Topic(r.URL.Path); topic != "" && rec.status < 400 {
			group, user := auth.CurrentGroup(r.Context(), sm), auth.CurrentUser(r.Context(), sm)
			hub.Publish(events.Event{Topic: topic, GroupID: group, UserID: user, ClientID: clientID(r)})
			// An item created in or moved to another space changes that space too (RF-83).
			if other, ok := otherSpace(r); ok && other != group {
				hub.Publish(events.Event{Topic: topic, GroupID: other, UserID: user, ClientID: clientID(r)})
			}
			g, err := accounts.GetGroup(r.Context(), group)
			switch {
			case err != nil:
			case g.Personal:
				// A write in the personal space, like paying the whole invoice, also changes what her groups show.
				if mine, err := accounts.GroupsOf(r.Context(), user); err == nil {
					for _, o := range mine {
						if !o.Personal {
							hub.Publish(events.Event{Topic: topic, GroupID: o.ID, UserID: user, ClientID: clientID(r)})
						}
					}
				}
			default:
				// A write in a group also changes the full views each member keeps in the personal space: the
				// invoice, the statement and the Alelo month of a holder (RF-83). Only the topic goes, never data.
				if ms, err := accounts.Members(r.Context(), group); err == nil {
					for _, m := range ms {
						theirs, err := accounts.GroupsOf(r.Context(), m.UserID)
						if err != nil {
							continue
						}
						for _, o := range theirs {
							if o.Personal {
								hub.Publish(events.Event{Topic: topic, GroupID: o.ID, UserID: user, ClientID: clientID(r)})
							}
						}
					}
				}
			}
		}
	})
}

// otherSpace returns the space a write also reached: the target of a move, or the X-Radar-Space of a creation.
// The move comes first because the client sends the active space in the header of every call. Only requests that
// passed the membership checks get here with a status below 400.
func otherSpace(r *http.Request) (int64, bool) {
	if rest, ok := strings.CutPrefix(r.URL.Path, "/api/spaces/"); ok {
		if id, tail, ok := strings.Cut(rest, "/"); ok && tail == "moves" {
			if n, err := strconv.ParseInt(id, 10, 64); err == nil && n > 0 {
				return n, true
			}
		}
	}
	if id, ok := spaceHeader(r); ok {
		return id, true
	}
	return 0, false
}
