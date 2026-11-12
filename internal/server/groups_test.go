package server

import (
	"encoding/json"
	"fmt"
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
		{name: "existing person twice", client: ana, method: http.MethodPost, path: "/api/group/members", body: `{"login":"bruno","pin":"7351","role":"member"}`, wantStatus: http.StatusConflict, wantCode: "group_already_member"},
		{name: "unknown login looks like a wrong PIN", client: ana, method: http.MethodPost, path: "/api/group/members", body: `{"login":"zeca","pin":"1234","role":"member"}`, wantStatus: http.StatusBadRequest, wantCode: "login_or_pin_wrong"},
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

// TestNoTakeoverAcrossGroups keeps an administrator of one group from reaching the login a person uses in another:
// joining needs the PIN of the person, and only whoever runs every group of the person resets it (RF-80).
func TestNoTakeoverAcrossGroups(t *testing.T) {
	srv, _ := testServer(t)
	ana := newClient(t)
	signIn(t, ana, srv, "ana")
	if resp := send(t, ana, srv, http.MethodPost, "/api/group/people", `{"login":"carla","name":"Carla Ficticia","pin":"9264","role":"member"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("register carla = %d", resp.StatusCode)
	}
	carla := newClient(t)
	if resp := postJSON(t, carla, srv, "/api/auth/login", `{"groupId":1,"userId":3,"pin":"9264"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("carla login = %d", resp.StatusCode)
	}
	// A member of Casa cannot create groups from it.
	if resp := send(t, carla, srv, http.MethodPost, "/api/groups", `{"name":"Praia"}`); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("member creates group = %d", resp.StatusCode)
	}
	// Ana creates the beach group and Carla joins it as administrator, typing her own PIN.
	resp := send(t, ana, srv, http.MethodPost, "/api/groups", `{"name":"Praia"}`)
	var beach struct{ ID int64 }
	_ = json.NewDecoder(resp.Body).Decode(&beach)
	if resp := send(t, ana, srv, http.MethodPut, "/api/me/group", fmt.Sprintf(`{"groupId":%d}`, beach.ID)); resp.StatusCode != http.StatusOK {
		t.Fatalf("switch = %d", resp.StatusCode)
	}
	if resp := send(t, ana, srv, http.MethodPost, "/api/group/members", `{"login":"carla","pin":"9264","role":"admin"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("carla joins the beach = %d", resp.StatusCode)
	}
	if resp := send(t, carla, srv, http.MethodPut, "/api/me/group", fmt.Sprintf(`{"groupId":%d}`, beach.ID)); resp.StatusCode != http.StatusOK {
		t.Fatalf("carla switches = %d", resp.StatusCode)
	}
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{name: "bringing Bruno without his PIN", method: http.MethodPost, path: "/api/group/members", body: `{"login":"bruno","pin":"0000","role":"member"}`, wantStatus: http.StatusBadRequest, wantCode: "login_or_pin_wrong"},
		{name: "resetting the PIN of Ana, who is also in Casa", method: http.MethodPut, path: "/api/group/members/1/pin", body: `{"pin":"1111"}`, wantStatus: http.StatusForbidden, wantCode: "pin_reset_other_groups"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := send(t, carla, srv, tt.method, tt.path, tt.body)
			var e struct{ Code string }
			_ = json.NewDecoder(resp.Body).Decode(&e)
			if resp.StatusCode != tt.wantStatus || e.Code != tt.wantCode {
				t.Fatalf("status = %d %s, want %d %s", resp.StatusCode, e.Code, tt.wantStatus, tt.wantCode)
			}
		})
	}
	// Ana still enters Casa with her own PIN.
	if resp := postJSON(t, newClient(t), srv, "/api/auth/login", `{"groupId":1,"userId":1,"pin":"4821"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("ana login after the attempts = %d", resp.StatusCode)
	}
}
