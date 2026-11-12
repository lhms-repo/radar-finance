// Package backup copies the database with VACUUM INTO on a daily schedule, at
// startup when the scheduled copy was missed and on demand (RNF-05 to RNF-07).
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Kind tells why a backup copy was made.
type Kind string

// Backup kinds, also used as file name suffixes.
const (
	KindDaily   Kind = "daily"
	KindStartup Kind = "startup"
	KindManual  Kind = "manual"
)

// nameLayout is the time layout used inside backup file names, in business time.
const nameLayout = "20060102-150405"

// namePattern matches the backup files written by this package.
var namePattern = regexp.MustCompile(`^radar-(\d{8}-\d{6})-(daily|startup|manual)\.db$`)

// Info describes one backup copy on disk.
type Info struct {
	Name      string
	CreatedAt time.Time
	SizeBytes int64
	Kind      Kind
}

// Options configures the backup service.
type Options struct {
	Dir  string
	Keep int
	Hour int
	Loc  *time.Location
}

// Status summarizes the backup state for the health page.
type Status struct {
	Last      *Info
	Next      time.Time
	LastError string
	ErrorAt   *time.Time
}

// Service makes and prunes backup copies.
type Service struct {
	conn    *sql.DB
	opts    Options
	log     *slog.Logger
	now     func() time.Time
	mu      sync.Mutex
	errMu   sync.Mutex
	lastErr error
	errAt   time.Time
	// next is when the schedule fires again, sooner than the daily time after a failure.
	next time.Time
}

// New creates the backup service.
func New(conn *sql.DB, opts Options, log *slog.Logger) *Service {
	return &Service{conn: conn, opts: opts, log: log, now: time.Now}
}

// Run writes a consistent copy of the database and prunes old copies.
func (s *Service) Run(ctx context.Context, kind Kind) (Info, error) {
	info, err := s.run(ctx, kind)
	s.errMu.Lock()
	if err != nil {
		s.lastErr, s.errAt = err, s.now()
	} else {
		s.lastErr = nil
	}
	s.errMu.Unlock()
	if err != nil {
		s.log.Error("backup failed", "kind", kind, "err", err)
		return Info{}, err
	}
	s.log.Info("backup done", "kind", kind, "file", info.Name, "bytes", info.SizeBytes)
	return info, nil
}

// run does the work of Run while holding the service lock.
func (s *Service) run(ctx context.Context, kind Kind) (Info, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.opts.Dir, 0o750); err != nil {
		return Info{}, fmt.Errorf("create backup folder: %w", err)
	}
	now := s.now().In(s.opts.Loc).Truncate(time.Second)
	name := fileName(now, kind, s.opts.Loc)
	final := filepath.Join(s.opts.Dir, name)
	if st, err := os.Stat(final); err == nil {
		return Info{Name: name, CreatedAt: now, SizeBytes: st.Size(), Kind: kind}, nil
	}
	tmp := final + ".tmp"
	_ = os.Remove(tmp)
	if _, err := s.conn.ExecContext(ctx, "VACUUM INTO ?", tmp); err != nil {
		// A partial copy has a name no later run reuses, so it would stay in the folder for good.
		_ = os.Remove(tmp)
		return Info{}, fmt.Errorf("vacuum into: %w", err)
	}
	if err := verify(ctx, tmp); err != nil {
		_ = os.Remove(tmp)
		return Info{}, err
	}
	if err := syncFile(tmp); err != nil {
		_ = os.Remove(tmp)
		return Info{}, err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return Info{}, fmt.Errorf("rename backup: %w", err)
	}
	st, err := os.Stat(final)
	if err != nil {
		return Info{}, err
	}
	if err := s.prune(); err != nil {
		s.log.Warn("prune backups", "err", err)
	}
	return Info{Name: name, CreatedAt: now, SizeBytes: st.Size(), Kind: kind}, nil
}

// List returns the backup copies, newest first.
func (s *Service) List() ([]Info, error) {
	entries, err := os.ReadDir(s.opts.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Info{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Info{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		at, kind, ok := parseFileName(e.Name(), s.opts.Loc)
		if !ok {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Info{Name: e.Name(), CreatedAt: at, SizeBytes: fi.Size(), Kind: kind})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// Status returns the last copy, the next scheduled time and the last error.
func (s *Service) Status() (Status, error) {
	list, err := s.List()
	if err != nil {
		return Status{}, err
	}
	st := Status{Next: nextScheduled(s.now(), s.opts.Hour, s.opts.Loc)}
	if len(list) > 0 {
		st.Last = &list[0]
	}
	s.errMu.Lock()
	if !s.next.IsZero() {
		st.Next = s.next
	}
	if s.lastErr != nil {
		at := s.errAt
		st.LastError, st.ErrorAt = s.lastErr.Error(), &at
	}
	s.errMu.Unlock()
	return st, nil
}

// Start runs the startup catch up and then the daily schedule until ctx is done.
func (s *Service) Start(ctx context.Context) {
	list, err := s.List()
	if err != nil {
		s.log.Error("list backups", "err", err)
	}
	var last *time.Time
	if len(list) > 0 {
		last = &list[0].CreatedAt
	}
	next := nextScheduled(s.now(), s.opts.Hour, s.opts.Loc)
	if needsCatchUp(s.now(), last, s.opts.Hour, s.opts.Loc) {
		_, err := s.Run(ctx, KindStartup)
		next = nextAfterRun(s.now(), err, s.opts.Hour, s.opts.Loc)
	}
	s.setNext(next)
	// A one minute tick, instead of a long timer, survives the host sleeping.
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := s.now()
			if now.Before(next) {
				continue
			}
			_, err := s.Run(ctx, KindDaily)
			next = nextAfterRun(s.now(), err, s.opts.Hour, s.opts.Loc)
			s.setNext(next)
		}
	}
}

// setNext records when the schedule fires again, for the health page.
func (s *Service) setNext(t time.Time) {
	s.errMu.Lock()
	s.next = t
	s.errMu.Unlock()
}

// retryDelay is how long the schedule waits before trying again after a failed copy.
const retryDelay = 15 * time.Minute

// nextAfterRun returns when the schedule fires again: the next daily time after a good copy, or soon after a failure.
func nextAfterRun(now time.Time, err error, hour int, loc *time.Location) time.Time {
	if err != nil {
		return now.Add(retryDelay)
	}
	return nextScheduled(now, hour, loc)
}

// prune deletes the copies beyond the retention limit.
func (s *Service) prune() error {
	list, err := s.List()
	if err != nil {
		return err
	}
	var errs []error
	for _, old := range toPrune(list, s.opts.Keep) {
		if err := os.Remove(filepath.Join(s.opts.Dir, old.Name)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// keepManual is how many manual copies are kept, apart from the daily ones.
const keepManual = 10

// toPrune returns the copies to delete from a newest first list: daily and startup copies beyond keep
// (RNF-05), and manual copies beyond keepManual, so tapping the button never pushes out the daily history.
func toPrune(list []Info, keep int) []Info {
	var out []Info
	var daily, manual int
	for _, b := range list {
		if b.Kind == KindManual {
			manual++
			if manual > keepManual {
				out = append(out, b)
			}
			continue
		}
		daily++
		if daily > keep {
			out = append(out, b)
		}
	}
	return out
}

// lastScheduled returns the most recent scheduled backup time at or before now.
func lastScheduled(now time.Time, hour int, loc *time.Location) time.Time {
	local := now.In(loc)
	at := time.Date(local.Year(), local.Month(), local.Day(), hour, 0, 0, 0, loc)
	if at.After(local) {
		at = at.AddDate(0, 0, -1)
	}
	return at
}

// nextScheduled returns the first scheduled backup time strictly after now.
func nextScheduled(now time.Time, hour int, loc *time.Location) time.Time {
	return lastScheduled(now, hour, loc).AddDate(0, 0, 1)
}

// needsCatchUp reports whether the last scheduled backup was missed (RNF-06).
func needsCatchUp(now time.Time, last *time.Time, hour int, loc *time.Location) bool {
	if last == nil {
		return true
	}
	return last.Before(lastScheduled(now, hour, loc))
}

// fileName builds the backup file name for a time and kind.
func fileName(at time.Time, kind Kind, loc *time.Location) string {
	return fmt.Sprintf("radar-%s-%s.db", at.In(loc).Format(nameLayout), kind)
}

// parseFileName extracts time and kind from a backup file name.
func parseFileName(name string, loc *time.Location) (time.Time, Kind, bool) {
	m := namePattern.FindStringSubmatch(name)
	if m == nil {
		return time.Time{}, "", false
	}
	at, err := time.ParseInLocation(nameLayout, m[1], loc)
	if err != nil {
		return time.Time{}, "", false
	}
	return at, Kind(m[2]), true
}

// verify opens the copy read only and runs a quick integrity check.
func verify(ctx context.Context, path string) error {
	conn, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open backup copy: %w", err)
	}
	defer conn.Close()
	var result string
	if err := conn.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil {
		return fmt.Errorf("check backup copy: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("backup copy failed the integrity check: %s", result)
	}
	return nil
}

// syncFile flushes a file to disk before it is renamed into place.
func syncFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync backup copy: %w", err)
	}
	return f.Close()
}
