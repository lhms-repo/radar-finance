package server

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/agenda"
	"github.com/italogfx/radar-finance/internal/alelo"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/backup"
	"github.com/italogfx/radar-finance/internal/bill"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/category"
	"github.com/italogfx/radar-finance/internal/config"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/financing"
	"github.com/italogfx/radar-finance/internal/importer"
	"github.com/italogfx/radar-finance/internal/income"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/market"
	"github.com/italogfx/radar-finance/internal/nfce"
	"github.com/italogfx/radar-finance/internal/notify"
	"github.com/italogfx/radar-finance/internal/receivable"
	"github.com/italogfx/radar-finance/internal/report"
	"github.com/italogfx/radar-finance/internal/savings"
	"github.com/italogfx/radar-finance/internal/settle"
	"github.com/italogfx/radar-finance/internal/space"
)

// testServer starts the full HTTP stack on a fresh database with one fictional user.
func testServer(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	conn, err := db.Open(filepath.Join(dir, "radar.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(context.Background(), conn, log); err != nil {
		t.Fatal(err)
	}
	old := auth.PINIterations
	auth.PINIterations = 1_000
	t.Cleanup(func() { auth.PINIterations = old })
	accounts := account.NewService(conn)
	ctx := context.Background()
	ids := map[string]int64{}
	for _, u := range [][2]string{{"ana", "Ana Ficticia"}, {"bruno", "Bruno Ficticio"}} {
		created, err := accounts.Create(ctx, u[0], u[1])
		if err != nil {
			t.Fatal(err)
		}
		ids[u[0]] = created.ID
		if err := accounts.SetPIN(ctx, created.ID, testPINs[u[0]]); err != nil {
			t.Fatal(err)
		}
	}
	home, err := accounts.CreateGroup(ctx, "Casa", ids["ana"])
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.AddMember(ctx, home.ID, ids["bruno"], account.RoleMember); err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation(config.Location)
	cfg := config.Config{DBPath: filepath.Join(dir, "radar.db"), BackupDir: filepath.Join(dir, "backups"), BackupKeep: 30, BackupHour: 3, Loc: loc}
	cards, bills, financings := card.NewService(conn), bill.NewService(conn), financing.NewService(conn)
	sav, reports := savings.NewService(conn), report.NewService(conn, cards)
	h := New(Deps{
		Config:        cfg,
		DB:            conn,
		Accounts:      accounts,
		Categories:    category.NewService(conn),
		Ledger:        ledger.NewService(conn),
		Spaces:        space.NewService(conn),
		Incomes:       income.NewService(conn),
		Cards:         cards,
		Settle:        settle.NewService(conn, cards),
		Bills:         bills,
		Financings:    financings,
		Agenda:        agenda.NewService(bills, financings, cards),
		Alelo:         alelo.NewService(conn),
		Receivables:   receivable.NewService(conn),
		Savings:       sav,
		Reports:       reports,
		Imports:       importer.NewService(conn, cards),
		Market:        market.NewService(conn).WithFetcher(nfce.FixtureClient{Dir: "../nfce/testdata"}),
		Notifications: notify.NewService(conn, agenda.NewService(bills, financings, cards), reports, sav, income.NewService(conn)),
		Sessions:      auth.NewSessionManager(conn, auth.SessionOptions{Idle: time.Hour, Lifetime: 24 * time.Hour}),
		Backups:       backup.New(conn, backup.Options{Dir: cfg.BackupDir, Keep: 30, Hour: 3, Loc: loc}, log),
		Version:       "test",
		StartedAt:     time.Now(),
		Log:           log,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, conn
}

// testPINs are the fictional PINs of the people of the test server.
var testPINs = map[string]string{"ana": "4821", "bruno": "7351"}

// testUserIDs are the ids the test server gives its people, created in this order.
var testUserIDs = map[string]int64{"ana": 1, "bruno": 2}

// signIn enters the group "Casa" of the test server as the person, with the right PIN (RF-01).
func signIn(t *testing.T, c *http.Client, srv *httptest.Server, login string) *http.Response {
	t.Helper()
	return postJSON(t, c, srv, "/api/auth/login", fmt.Sprintf(`{"groupId":1,"userId":%d,"pin":%q}`, testUserIDs[login], testPINs[login]))
}

// newClient returns an HTTP client that keeps cookies.
func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

// get sends a GET request and fails the test on transport errors.
func get(t *testing.T, c *http.Client, url string) *http.Response {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// postJSON sends a JSON body like the browser does, with a same origin header.
func postJSON(t *testing.T, c *http.Client, srv *httptest.Server, path, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srv.URL)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// TestLoginFlow covers entering by group, person and PIN, session use, switching groups and logout (RF-01, RF-81, RNF-10).
func TestLoginFlow(t *testing.T) {
	srv, conn := testServer(t)
	c := newClient(t)

	resp := get(t, c, srv.URL+"/api/me")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me without session = %d, want 401", resp.StatusCode)
	}

	resp = get(t, c, srv.URL+"/api/auth/groups")
	var groups []struct {
		ID   int64
		Name string
	}
	_ = json.NewDecoder(resp.Body).Decode(&groups)
	if resp.StatusCode != http.StatusOK || len(groups) != 1 || groups[0].Name != "Casa" {
		t.Fatalf("login groups = %d %+v", resp.StatusCode, groups)
	}
	resp = get(t, c, srv.URL+"/api/auth/groups/1/members")
	var members []struct {
		UserID int64 `json:"userId"`
		Name   string
		HasPIN bool `json:"hasPin"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&members)
	if resp.StatusCode != http.StatusOK || len(members) != 2 || members[0].Name != "Ana Ficticia" || !members[0].HasPIN {
		t.Fatalf("login members = %d %+v", resp.StatusCode, members)
	}
	// The owner of a personal space is not listed through it (RF-83).
	if resp := get(t, c, srv.URL+"/api/auth/groups/2/members"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("members of a personal space = %d, want 404", resp.StatusCode)
	}
	if resp := get(t, c, srv.URL+"/api/auth/groups/99/members"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("members of a missing group = %d, want 404", resp.StatusCode)
	}

	tests := []struct {
		name string
		body string
		want int
		code string
	}{
		{name: "unknown person", body: `{"groupId":1,"userId":99,"pin":"4821"}`, want: http.StatusUnauthorized},
		{name: "wrong PIN", body: `{"groupId":1,"userId":1,"pin":"0000"}`, want: http.StatusBadRequest, code: "pin_wrong"},
		{name: "own personal space", body: `{"groupId":2,"userId":1,"pin":"4821"}`, want: http.StatusBadRequest, code: "group_not_member"},
		{name: "personal space of someone else", body: `{"groupId":3,"userId":1,"pin":"4821"}`, want: http.StatusBadRequest, code: "group_not_member"},
		{name: "no group", body: `{"userId":1,"pin":"4821"}`, want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := postJSON(t, c, srv, "/api/auth/login", tt.body)
			var e struct{ Code string }
			_ = json.NewDecoder(resp.Body).Decode(&e)
			if resp.StatusCode != tt.want || (tt.code != "" && e.Code != tt.code) {
				t.Fatalf("login = %d %s, want %d %s", resp.StatusCode, e.Code, tt.want, tt.code)
			}
		})
	}

	resp = signIn(t, c, srv, "ana")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login = %d, want 200", resp.StatusCode)
	}
	var me struct {
		Login, Name, Theme string
		Group              struct {
			ID   int64
			Name string
			Role string
		}
		Groups []struct{ ID int64 }
	}
	_ = json.NewDecoder(resp.Body).Decode(&me)
	if me.Name != "Ana Ficticia" || me.Theme != "system" || me.Group.Name != "Casa" || me.Group.Role != "admin" || len(me.Groups) != 2 {
		t.Fatalf("unexpected me: %+v", me)
	}
	cookie := resp.Header.Get("Set-Cookie")
	if !strings.Contains(cookie, "radar_session=") || !strings.Contains(cookie, "HttpOnly") {
		t.Fatalf("session cookie missing or not HttpOnly: %s", cookie)
	}

	resp = get(t, c, srv.URL+"/api/me")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me with session = %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("api responses must not be cached")
	}

	// A second group of Ana: switching needs no PIN, and a group she is not in is refused.
	if _, err := conn.Exec(`INSERT INTO groups (id, name, created_at) VALUES (10, 'Praia', '2026-11-12 12:00:00+00:00'), (11, 'Outro', '2026-11-12 12:00:00+00:00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO group_members (group_id, person_id, role, weight_bp, joined_at) VALUES (10, 1, 'member', 10000, '2026-11-12 12:00:00+00:00')`); err != nil {
		t.Fatal(err)
	}
	put := func(body string) *http.Response {
		req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/me/group", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", srv.URL)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	if resp := put(`{"groupId":11}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("switch to a group of others = %d, want 400", resp.StatusCode)
	}
	// The personal space of Bruno is closed to Ana; her own opens like any group (RF-83).
	if resp := put(`{"groupId":3}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("switch to the personal space of bruno = %d, want 400", resp.StatusCode)
	}
	resp = put(`{"groupId":2}`)
	_ = json.NewDecoder(resp.Body).Decode(&me)
	if resp.StatusCode != http.StatusOK || me.Group.Name != "Pessoal" {
		t.Fatalf("switch to her personal space = %d %+v", resp.StatusCode, me)
	}
	resp = put(`{"groupId":10}`)
	_ = json.NewDecoder(resp.Body).Decode(&me)
	if resp.StatusCode != http.StatusOK || me.Group.Name != "Praia" || me.Group.Role != "member" || len(me.Groups) != 3 {
		t.Fatalf("switch = %d %+v", resp.StatusCode, me)
	}
	resp = get(t, c, srv.URL+"/api/me")
	_ = json.NewDecoder(resp.Body).Decode(&me)
	if me.Group.Name != "Praia" {
		t.Fatalf("active group after the switch = %+v", me.Group)
	}

	resp = postJSON(t, c, srv, "/api/auth/logout", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", resp.StatusCode)
	}
	resp = get(t, c, srv.URL+"/api/me")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout = %d, want 401", resp.StatusCode)
	}
}

// TestCrossOriginBlocked checks that another site cannot post with the session.
func TestCrossOriginBlocked(t *testing.T) {
	srv, _ := testServer(t)
	c := newClient(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/auth/login", strings.NewReader(`{"login":"ana"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

// TestBackupAndExport checks manual backup, health and the CSV export (RNF-07, RNF-18).
func TestBackupAndExport(t *testing.T) {
	srv, _ := testServer(t)
	c := newClient(t)
	signIn(t, c, srv, "ana")

	resp := postJSON(t, c, srv, "/api/system/backups", "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("backup = %d, want 201", resp.StatusCode)
	}

	resp = get(t, c, srv.URL+"/api/system/health")
	var health struct {
		LastBackup *struct{ Kind string }
		Version    string
	}
	_ = json.NewDecoder(resp.Body).Decode(&health)
	resp.Body.Close()
	if health.LastBackup == nil || health.LastBackup.Kind != "manual" || health.Version != "test" {
		t.Fatalf("unexpected health: %+v", health)
	}

	resp = get(t, c, srv.URL+"/api/system/export")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("export = %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		names[f.Name] = string(data)
	}
	if _, ok := names["sessions.csv"]; ok {
		t.Fatal("sessions must not be exported")
	}
	users, ok := names["users.csv"]
	if !ok {
		t.Fatal("users.csv missing")
	}
	if !strings.Contains(users, "bruno") {
		t.Fatal("users.csv missing data")
	}
	if !strings.Contains(names["persons.csv"], "Ana Ficticia") {
		t.Fatal("persons.csv missing data")
	}
}

// TestUnknownAPIRoute checks the JSON 404 for unknown API paths.
func TestUnknownAPIRoute(t *testing.T) {
	srv, _ := testServer(t)
	c := newClient(t)
	signIn(t, c, srv, "ana")
	resp := get(t, c, srv.URL+"/api/nada")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
