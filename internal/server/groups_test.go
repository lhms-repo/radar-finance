package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestGroupRoutes lets administrators manage the active group and its people, and refuses the others (RF-79, RF-80, RN-27).
func TestGroupRoutes(t *testing.T) {
	srv, _ := testServer(t)
	ana, bruno := newClient(t), newClient(t)
	signIn(t, ana, srv, "ana")
	signIn(t, bruno, srv, "bruno")

	tests := []struct {
		name       string
		client     *http.Client
		method     string
		path       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{name: "members see the group", client: bruno, method: http.MethodGet, path: "/api/group", wantStatus: http.StatusOK},
		{name: "a member cannot rename", client: bruno, method: http.MethodPut, path: "/api/group", body: `{"name":"Lar"}`, wantStatus: http.StatusForbidden, wantCode: "group_admin_only"},
		{name: "a member cannot register people", client: bruno, method: http.MethodPost, path: "/api/group/people", body: `{"login":"x","name":"X","pin":"1234","role":"member"}`, wantStatus: http.StatusForbidden},
		{name: "a member cannot reset a PIN", client: bruno, method: http.MethodPut, path: "/api/group/members/1/pin", body: `{"pin":"1111"}`, wantStatus: http.StatusForbidden},
		{name: "a member cannot create groups", client: bruno, method: http.MethodPost, path: "/api/groups", body: `{"name":"Outro"}`, wantStatus: http.StatusForbidden},
		{name: "admin renames", client: ana, method: http.MethodPut, path: "/api/group", body: `{"name":"Lar"}`, wantStatus: http.StatusOK},
		{name: "admin registers Carla", client: ana, method: http.MethodPost, path: "/api/group/people", body: `{"login":"carla","name":"Carla Ficticia","pin":"9264","role":"member"}`, wantStatus: http.StatusOK},
		{name: "login already taken", client: ana, method: http.MethodPost, path: "/api/group/people", body: `{"login":"bruno","name":"Outro","pin":"1234","role":"member"}`, wantStatus: http.StatusConflict, wantCode: "login_taken"},
		{name: "existing person twice", client: ana, method: http.MethodPost, path: "/api/group/members", body: `{"login":"bruno","role":"member"}`, wantStatus: http.StatusConflict, wantCode: "group_already_member"},
		{name: "unknown login", client: ana, method: http.MethodPost, path: "/api/group/members", body: `{"login":"zeca","role":"member"}`, wantStatus: http.StatusNotFound},
		{name: "admin promotes Bruno", client: ana, method: http.MethodPut, path: "/api/group/members/2", body: `{"role":"admin"}`, wantStatus: http.StatusOK},
		{name: "admin resets the PIN of Bruno", client: ana, method: http.MethodPut, path: "/api/group/members/2/pin", body: `{"pin":"2468"}`, wantStatus: http.StatusNoContent},
		{name: "short PIN", client: ana, method: http.MethodPut, path: "/api/group/members/2/pin", body: `{"pin":"12"}`, wantStatus: http.StatusBadRequest},
		{name: "PIN of someone outside the group", client: ana, method: http.MethodPut, path: "/api/group/members/99/pin", body: `{"pin":"1111"}`, wantStatus: http.StatusBadRequest, wantCode: "group_not_member"},
		{name: "admin creates another group", client: ana, method: http.MethodPost, path: "/api/groups", body: `{"name":"Praia"}`, wantStatus: http.StatusCreated},
		{name: "group name taken", client: ana, method: http.MethodPost, path: "/api/groups", body: `{"name":"praia"}`, wantStatus: http.StatusConflict, wantCode: "group_name_taken"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := send(t, tt.client, srv, tt.method, tt.path, tt.body)
			var e struct{ Code string }
			if resp.StatusCode >= 400 {
				_ = json.NewDecoder(resp.Body).Decode(&e)
			}
			if resp.StatusCode != tt.wantStatus || (tt.wantCode != "" && e.Code != tt.wantCode) {
				t.Fatalf("status = %d %s, want %d %s", resp.StatusCode, e.Code, tt.wantStatus, tt.wantCode)
			}
		})
	}

	resp := send(t, ana, srv, http.MethodGet, "/api/group", "")
	var g struct {
		Name    string
		Members []struct {
			Login    string
			Role     string
			WeightBp int64 `json:"weightBp"`
			HasPin   bool  `json:"hasPin"`
		}
	}
	_ = json.NewDecoder(resp.Body).Decode(&g)
	if g.Name != "Lar" || len(g.Members) != 3 || g.Members[1].Role != "admin" || g.Members[2].Login != "carla" || !g.Members[2].HasPin {
		t.Fatalf("group = %+v", g)
	}
	var total int64
	for _, m := range g.Members {
		total += m.WeightBp
	}
	if total != 10_000 {
		t.Fatalf("weights add to %d, want 10000", total)
	}

	// Carla enters with her PIN, and Bruno with the new one; the old PIN of Bruno no longer works.
	carla := newClient(t)
	if resp := postJSON(t, carla, srv, "/api/auth/login", `{"groupId":1,"userId":3,"pin":"9264"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("carla login = %d", resp.StatusCode)
	}
	if resp := postJSON(t, newClient(t), srv, "/api/auth/login", `{"groupId":1,"userId":2,"pin":"7351"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("old PIN of bruno = %d, want 400", resp.StatusCode)
	}
	if resp := postJSON(t, newClient(t), srv, "/api/auth/login", `{"groupId":1,"userId":2,"pin":"2468"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("new PIN of bruno = %d", resp.StatusCode)
	}
	// Ana is the administrator of the new group and can switch to it.
	resp = send(t, ana, srv, http.MethodGet, "/api/me", "")
	var me struct{ Groups []struct{ Name, Role string } }
	_ = json.NewDecoder(resp.Body).Decode(&me)
	if len(me.Groups) != 2 || me.Groups[1].Name != "Praia" || me.Groups[1].Role != "admin" {
		t.Fatalf("groups of ana = %+v", me.Groups)
	}
}
