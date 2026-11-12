package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/italogfx/radar-finance/internal/db"
)

// saoPaulo loads the business time zone for the tests.
func saoPaulo(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// TestLastScheduled checks the most recent scheduled backup time.
func TestLastScheduled(t *testing.T) {
	loc := saoPaulo(t)
	at := func(d, h, m int) time.Time { return time.Date(2026, 11, d, h, m, 0, 0, loc) }
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{name: "after the hour, same day", now: at(10, 9, 0), want: at(10, 3, 0)},
		{name: "before the hour, previous day", now: at(10, 2, 59), want: at(9, 3, 0)},
		{name: "exactly at the hour", now: at(10, 3, 0), want: at(10, 3, 0)},
		{name: "now given in UTC", now: at(10, 9, 0).UTC(), want: at(10, 3, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lastScheduled(tt.now, 3, loc)
			if !got.Equal(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNextScheduled checks the next scheduled backup time.
func TestNextScheduled(t *testing.T) {
	loc := saoPaulo(t)
	at := func(d, h, m int) time.Time { return time.Date(2026, 11, d, h, m, 0, 0, loc) }
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{name: "before the hour, same day", now: at(10, 2, 0), want: at(10, 3, 0)},
		{name: "after the hour, next day", now: at(10, 9, 0), want: at(11, 3, 0)},
		{name: "exactly at the hour, next day", now: at(10, 3, 0), want: at(11, 3, 0)},
		{name: "end of month", now: time.Date(2026, 11, 30, 23, 0, 0, 0, loc), want: time.Date(2026, 12, 1, 3, 0, 0, 0, loc)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextScheduled(tt.now, 3, loc)
			if !got.Equal(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNextAfterRun retries soon after a failed copy instead of waiting for the next day (RNF-06).
func TestNextAfterRun(t *testing.T) {
	loc := saoPaulo(t)
	now := time.Date(2026, 11, 10, 3, 0, 30, 0, loc)
	tests := []struct {
		name string
		err  error
		want time.Time
	}{
		{name: "good copy, next day", want: time.Date(2026, 11, 11, 3, 0, 0, 0, loc)},
		{name: "failed copy, retry soon", err: errors.New("disk unavailable"), want: now.Add(retryDelay)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextAfterRun(now, tt.err, 3, loc); !got.Equal(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNeedsCatchUp checks the startup backup when the scheduled one was missed (RNF-06).
func TestNeedsCatchUp(t *testing.T) {
	loc := saoPaulo(t)
	at := func(d, h, m int) time.Time { return time.Date(2026, 11, d, h, m, 0, 0, loc) }
	ptr := func(t time.Time) *time.Time { return &t }
	tests := []struct {
		name string
		now  time.Time
		last *time.Time
		want bool
	}{
		{name: "no backup yet", now: at(10, 9, 0), last: nil, want: true},
		{name: "machine was off at 3h today", now: at(10, 9, 0), last: ptr(at(9, 3, 0)), want: true},
		{name: "today's backup ran", now: at(10, 9, 0), last: ptr(at(10, 3, 0)), want: false},
		{name: "before 3h with yesterday's backup", now: at(10, 2, 0), last: ptr(at(9, 3, 0)), want: false},
		{name: "before 3h and yesterday was missed", now: at(10, 2, 0), last: ptr(at(8, 3, 0)), want: true},
		{name: "manual backup before today's schedule", now: at(10, 9, 0), last: ptr(at(10, 1, 0)), want: true},
		{name: "manual backup after today's schedule", now: at(10, 9, 0), last: ptr(at(10, 8, 0)), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsCatchUp(tt.now, tt.last, 3, loc); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFileNameRoundTrip checks that backup names encode time and kind.
func TestFileNameRoundTrip(t *testing.T) {
	loc := saoPaulo(t)
	tests := []struct {
		at   time.Time
		kind Kind
		name string
	}{
		{at: time.Date(2026, 11, 10, 3, 0, 5, 0, loc), kind: KindDaily, name: "radar-20261110-030005-daily.db"},
		{at: time.Date(2026, 1, 2, 14, 30, 0, 0, loc), kind: KindManual, name: "radar-20260102-143000-manual.db"},
		{at: time.Date(2026, 1, 2, 8, 0, 0, 0, loc), kind: KindStartup, name: "radar-20260102-080000-startup.db"},
	}
	for _, tt := range tests {
		got := fileName(tt.at, tt.kind, loc)
		if got != tt.name {
			t.Fatalf("fileName = %s, want %s", got, tt.name)
		}
		at, kind, ok := parseFileName(got, loc)
		if !ok || !at.Equal(tt.at) || kind != tt.kind {
			t.Fatalf("parseFileName(%s) = %v %v %v", got, at, kind, ok)
		}
	}
	for _, bad := range []string{"radar.db", "radar-20261110-030005-daily.db-wal", "radar-2026-daily.db", "radar-20261110-030005-other.db", "notes.txt"} {
		if _, _, ok := parseFileName(bad, loc); ok {
			t.Fatalf("parseFileName(%s) should fail", bad)
		}
	}
}

// TestToPruneKinds keeps the 30 daily copies whatever the manual ones do, and only the newest manual copies (RNF-05, RNF-07).
func TestToPruneKinds(t *testing.T) {
	base := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	// mixed returns the copies newest first: manual taps on top of a month of daily and startup copies.
	mixed := func(manual, daily int) []Info {
		var out []Info
		for i := range manual {
			out = append(out, Info{Name: fmt.Sprintf("manual-%02d", i), Kind: KindManual, CreatedAt: base.AddDate(0, 2, -i)})
		}
		for i := range daily {
			kind := KindDaily
			if i%10 == 0 {
				kind = KindStartup
			}
			out = append(out, Info{Name: fmt.Sprintf("daily-%02d", i), Kind: kind, CreatedAt: base.AddDate(0, 0, daily-i)})
		}
		return out
	}
	tests := []struct {
		name       string
		manual     int
		daily      int
		wantManual int
		wantDaily  int
	}{
		{name: "many manual taps keep the daily history", manual: 35, daily: 30, wantManual: 25},
		{name: "few manual copies", manual: 3, daily: 31, wantDaily: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var manual, daily int
			for _, p := range toPrune(mixed(tt.manual, tt.daily), 30) {
				if p.Kind == KindManual {
					manual++
					if p.Name < fmt.Sprintf("manual-%02d", keepManual) {
						t.Fatalf("pruned a newest manual copy: %s", p.Name)
					}
				} else {
					daily++
				}
			}
			if manual != tt.wantManual || daily != tt.wantDaily {
				t.Fatalf("pruned %d manual and %d daily, want %d and %d", manual, daily, tt.wantManual, tt.wantDaily)
			}
		})
	}
}

// TestToPrune checks that only the newest copies are kept (RNF-05).
func TestToPrune(t *testing.T) {
	base := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	build := func(n int) []Info {
		out := make([]Info, n)
		for i := range out {
			// Newest first, as returned by List.
			out[i] = Info{Name: string(rune('a' + i)), CreatedAt: base.AddDate(0, 0, n-i)}
		}
		return out
	}
	tests := []struct {
		name  string
		count int
		keep  int
		want  int
	}{
		{name: "fewer than keep", count: 5, keep: 30, want: 0},
		{name: "exactly keep", count: 30, keep: 30, want: 0},
		{name: "two over keep", count: 32, keep: 30, want: 2},
		{name: "keep one", count: 3, keep: 1, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := build(tt.count)
			got := toPrune(list, tt.keep)
			if len(got) != tt.want {
				t.Fatalf("pruned %d, want %d", len(got), tt.want)
			}
			for _, p := range got {
				for _, k := range list[:tt.keep] {
					if p.Name == k.Name {
						t.Fatalf("pruned a copy that should be kept: %s", p.Name)
					}
				}
			}
		})
	}
}

// TestRunCreatesConsistentCopy runs a real VACUUM INTO and checks retention.
func TestRunCreatesConsistentCopy(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.Open(filepath.Join(dir, "data", "radar.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := db.Migrate(context.Background(), conn, log); err != nil {
		t.Fatal(err)
	}

	loc := saoPaulo(t)
	now := time.Date(2026, 11, 10, 3, 0, 0, 0, loc)
	backupDir := filepath.Join(dir, "backups")
	svc := New(conn, Options{Dir: backupDir, Keep: 2, Hour: 3, Loc: loc}, log)
	svc.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if _, err := svc.Run(context.Background(), KindDaily); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Minute)
	}
	list, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("kept %d copies, want 2", len(list))
	}
	if list[0].CreatedAt.Before(list[1].CreatedAt) {
		t.Fatal("list must be newest first")
	}

	copyConn, err := sql.Open("sqlite", "file:"+filepath.Join(backupDir, list[0].Name)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer copyConn.Close()
	var n int
	if err := copyConn.QueryRow("SELECT count(*) FROM sqlite_master WHERE name = 'users'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("backup copy is missing the users table: n=%d err=%v", n, err)
	}
	entries, _ := os.ReadDir(backupDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("temporary file left behind: %s", e.Name())
		}
	}
}
