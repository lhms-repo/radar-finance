package server

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
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
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/backup"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/category"
	"github.com/italogfx/radar-finance/internal/config"
	"github.com/italogfx/radar-finance/internal/db"
	"github.com/italogfx/radar-finance/internal/income"
	"github.com/italogfx/radar-finance/internal/ledger"
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
	accounts := account.NewService(conn)
	for _, u := range [][2]string{{"ana", "Ana Ficticia"}, {"bruno", "Bruno Ficticio"}} {
		if _, err := accounts.Create(context.Background(), u[0], u[1]); err != nil {
			t.Fatal(err)
		}
	}
	loc, _ := time.LoadLocation(config.Location)
	cfg := config.Config{DBPath: filepath.Join(dir, "radar.db"), BackupDir: filepath.Join(dir, "backups"), BackupKeep: 30, BackupHour: 3, Loc: loc}
	h := New(Deps{
		Config:     cfg,
		DB:         conn,
		Accounts:   accounts,
		Categories: category.NewService(conn),
		Ledger:     ledger.NewService(conn),
		Incomes:    income.NewService(conn),
		Cards:      card.NewService(conn),
		Sessions:   auth.NewSessionManager(conn, auth.SessionOptions{Idle: time.Hour, Lifetime: 24 * time.Hour}),
		Backups:    backup.New(conn, backup.Options{Dir: cfg.BackupDir, Keep: 30, Hour: 3, Loc: loc}, log),
		Version:    "test",
		StartedAt:  time.Now(),
		Log:        log,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, conn
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

// TestLoginFlow covers login, session use and logout (RF-01, RNF-10).
func TestLoginFlow(t *testing.T) {
	srv, _ := testServer(t)
	c := newClient(t)

	resp := get(t, c, srv.URL+"/api/me")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me without session = %d, want 401", resp.StatusCode)
	}

	resp = get(t, c, srv.URL+"/api/auth/users")
	var users []struct{ Login, Name string }
	_ = json.NewDecoder(resp.Body).Decode(&users)
	if resp.StatusCode != http.StatusOK || len(users) != 2 || users[0].Name != "Ana Ficticia" {
		t.Fatalf("login users = %d %+v", resp.StatusCode, users)
	}

	resp = postJSON(t, c, srv, "/api/auth/login", `{"login":"carla"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown user = %d, want 401", resp.StatusCode)
	}

	resp = postJSON(t, c, srv, "/api/auth/login", `{"login":"ANA"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login = %d, want 200", resp.StatusCode)
	}
	var me struct{ Login, Name, Theme string }
	_ = json.NewDecoder(resp.Body).Decode(&me)
	if me.Name != "Ana Ficticia" || me.Theme != "system" {
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
	postJSON(t, c, srv, "/api/auth/login", `{"login":"ana"}`)

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
	postJSON(t, c, srv, "/api/auth/login", `{"login":"ana"}`)
	resp := get(t, c, srv.URL+"/api/nada")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
