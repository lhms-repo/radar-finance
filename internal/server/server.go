// Package server wires the HTTP routes, middleware and static frontend.
package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"runtime/debug"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/agenda"
	"github.com/italogfx/radar-finance/internal/alelo"
	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/backup"
	"github.com/italogfx/radar-finance/internal/bill"
	"github.com/italogfx/radar-finance/internal/card"
	"github.com/italogfx/radar-finance/internal/category"
	"github.com/italogfx/radar-finance/internal/config"
	"github.com/italogfx/radar-finance/internal/financing"
	"github.com/italogfx/radar-finance/internal/income"
	"github.com/italogfx/radar-finance/internal/ledger"
	"github.com/italogfx/radar-finance/internal/problem"
)

// maxBodyBytes limits JSON request bodies.
const maxBodyBytes = 1 << 20

// publicRoutes do not require a logged in user.
var publicRoutes = map[string]bool{
	"GET /api/ping":        true,
	"GET /api/auth/users":  true,
	"POST /api/auth/login": true,
}

// contentSecurityPolicy only allows resources from the same origin.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; manifest-src 'self'; " +
	"frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

// Deps holds everything the HTTP layer needs.
type Deps struct {
	Config     config.Config
	DB         *sql.DB
	Accounts   *account.Service
	Categories *category.Service
	Ledger     *ledger.Service
	Incomes    *income.Service
	Cards      *card.Service
	Bills      *bill.Service
	Financings *financing.Service
	Agenda     *agenda.Service
	Alelo      *alelo.Service
	Sessions   *scs.SessionManager
	Backups    *backup.Service
	Version    string
	StartedAt  time.Time
	Log        *slog.Logger
	// Assets is the built frontend, or nil when Vite serves it in development.
	Assets fs.FS
}

// New returns the root HTTP handler.
func New(d Deps) http.Handler {
	api := &API{d: d}
	strict := gen.NewStrictHandlerWithOptions(api, nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadRequest, "invalid_request", "Dados inválidos.")
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			if p, ok := problem.As(err); ok {
				writeError(w, problemStatus(p.Kind), p.Code, p.Message)
				return
			}
			d.Log.Error("api error", "method", r.Method, "path", r.URL.Path, "err", err)
			writeError(w, http.StatusInternalServerError, "internal", "Algo deu errado. Tente de novo.")
		},
	})
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "Rota não encontrada.")
	})
	apiHandler := gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseURL:    "/api",
		BaseRouter: apiMux,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadRequest, "invalid_request", "Parâmetros inválidos.")
		},
	})

	protected := limitBody(auth.RequireUser(d.Sessions, publicRoutes, apiHandler))
	root := http.NewServeMux()
	root.Handle("/api/", http.NewCrossOriginProtection().Handler(d.Sessions.LoadAndSave(noStore(protected))))
	root.Handle("/", staticHandler(d.Assets))
	return securityHeaders(recoverer(d.Log, logRequests(d.Log, root)))
}

// problemStatus maps a business error kind to its HTTP status.
func problemStatus(k problem.Kind) int {
	switch k {
	case problem.KindNotFound:
		return http.StatusNotFound
	case problem.KindConflict:
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}

// writeError writes the JSON error body used by every API failure.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(gen.Error{Code: code, Message: message})
}

// limitBody caps the request body size.
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

// noStore keeps API responses out of browser and proxy caches.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// securityHeaders adds the browser hardening headers to every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(self), geolocation=(), microphone=()")
		next.ServeHTTP(w, r)
	})
}

// recoverer turns a panic into a 500 response and logs the stack.
func recoverer(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v)
				}
				log.Error("panic", "value", v, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal", "Algo deu errado. Tente de novo.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the response status for the request log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader records the status before writing it.
func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the original writer to http.ResponseController.
func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

// logRequests logs every API request with status and duration.
func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/ping" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(start))
	})
}

// staticHandler serves the embedded frontend, falling back to index.html for app routes.
func staticHandler(assets fs.FS) http.Handler {
	if assets == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("Frontend não embutido neste binário. Em desenvolvimento, abra o Vite na porta 5173.\n"))
		})
	}
	files := http.FileServerFS(assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if st, err := fs.Stat(assets, name); err != nil || st.IsDir() {
			// Client side route: serve the app shell.
			name = "index.html"
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}
