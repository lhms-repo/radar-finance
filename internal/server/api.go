package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/backup"
	"github.com/italogfx/radar-finance/internal/export"
)

// API implements the strict OpenAPI server interface.
type API struct {
	d Deps
}

var _ gen.StrictServerInterface = (*API)(nil)

// Ping answers the liveness check.
func (a *API) Ping(_ context.Context, _ gen.PingRequestObject) (gen.PingResponseObject, error) {
	return gen.Ping200JSONResponse{Status: "ok"}, nil
}

// ListLoginUsers lists the users shown on the "who is using" screen (RF-01).
func (a *API) ListLoginUsers(ctx context.Context, _ gen.ListLoginUsersRequestObject) (gen.ListLoginUsersResponseObject, error) {
	users, err := a.d.Accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListLoginUsers200JSONResponse, 0, len(users))
	for _, u := range users {
		out = append(out, gen.LoginUser{Login: u.Login, Name: u.Name})
	}
	return out, nil
}

// Login starts a session for the chosen user; there are no passwords (RF-01).
func (a *API) Login(ctx context.Context, req gen.LoginRequestObject) (gen.LoginResponseObject, error) {
	if req.Body == nil || req.Body.Login == "" {
		return gen.Login400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse{
			Code: "invalid_request", Message: "Escolha quem está usando.",
		}}, nil
	}
	user, err := a.d.Accounts.GetByLogin(ctx, req.Body.Login)
	if errors.Is(err, account.ErrNotFound) {
		return gen.Login401JSONResponse{UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse{
			Code: "unknown_user", Message: "Usuário não encontrado.",
		}}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := auth.SignIn(ctx, a.d.Sessions, user.ID); err != nil {
		return nil, err
	}
	a.d.Log.Info("login", "user", user.Login)
	return gen.Login200JSONResponse(toMe(user)), nil
}

// Logout destroys the session.
func (a *API) Logout(ctx context.Context, _ gen.LogoutRequestObject) (gen.LogoutResponseObject, error) {
	if err := auth.SignOut(ctx, a.d.Sessions); err != nil {
		return nil, err
	}
	return gen.Logout204Response{}, nil
}

// GetMe returns the logged in user.
func (a *API) GetMe(ctx context.Context, _ gen.GetMeRequestObject) (gen.GetMeResponseObject, error) {
	user, err := a.d.Accounts.Get(ctx, auth.UserID(ctx))
	if errors.Is(err, account.ErrNotFound) {
		_ = auth.SignOut(ctx, a.d.Sessions)
		return gen.GetMe401JSONResponse{UnauthorizedJSONResponse: unauthenticated()}, nil
	}
	if err != nil {
		return nil, err
	}
	return gen.GetMe200JSONResponse(toMe(user)), nil
}

// UpdatePreferences saves theme and haptics for the logged in user.
func (a *API) UpdatePreferences(ctx context.Context, req gen.UpdatePreferencesRequestObject) (gen.UpdatePreferencesResponseObject, error) {
	if req.Body == nil {
		return gen.UpdatePreferences400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse{
			Code: "invalid_request", Message: "Nenhuma preferência enviada.",
		}}, nil
	}
	prefs := account.Preferences{Haptics: req.Body.Haptics}
	if req.Body.Theme != nil {
		if !req.Body.Theme.Valid() {
			return gen.UpdatePreferences400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse{
				Code: "invalid_theme", Message: "Tema inválido.",
			}}, nil
		}
		theme := string(*req.Body.Theme)
		prefs.Theme = &theme
	}
	user, err := a.d.Accounts.UpdatePreferences(ctx, auth.UserID(ctx), prefs)
	if errors.Is(err, account.ErrNotFound) {
		return gen.UpdatePreferences401JSONResponse{UnauthorizedJSONResponse: unauthenticated()}, nil
	}
	if err != nil {
		return nil, err
	}
	return gen.UpdatePreferences200JSONResponse(toMe(user)), nil
}

// GetHealth reports version, uptime, database size and backup state (section 8.5).
func (a *API) GetHealth(_ context.Context, _ gen.GetHealthRequestObject) (gen.GetHealthResponseObject, error) {
	status, err := a.d.Backups.Status()
	if err != nil {
		return nil, err
	}
	h := gen.Health{
		Version:       a.d.Version,
		StartedAt:     a.d.StartedAt,
		UptimeSeconds: int64(time.Since(a.d.StartedAt).Seconds()),
		DbSizeBytes:   dbSize(a.d.Config.DBPath),
		NextBackupAt:  status.Next,
		BackupKeep:    a.d.Config.BackupKeep,
	}
	if status.Last != nil {
		b := toBackup(*status.Last)
		h.LastBackup = &b
	}
	if status.LastError != "" {
		h.BackupError = &status.LastError
		h.BackupErrorAt = status.ErrorAt
	}
	return gen.GetHealth200JSONResponse(h), nil
}

// ListBackups lists the backup copies, newest first (RNF-05).
func (a *API) ListBackups(_ context.Context, _ gen.ListBackupsRequestObject) (gen.ListBackupsResponseObject, error) {
	list, err := a.d.Backups.List()
	if err != nil {
		return nil, err
	}
	out := make(gen.ListBackups200JSONResponse, 0, len(list))
	for _, b := range list {
		out = append(out, toBackup(b))
	}
	return out, nil
}

// CreateBackup runs a manual backup (RNF-07).
func (a *API) CreateBackup(ctx context.Context, _ gen.CreateBackupRequestObject) (gen.CreateBackupResponseObject, error) {
	// A phone leaving the page must not cut the copy halfway.
	info, err := a.d.Backups.Run(context.WithoutCancel(ctx), backup.KindManual)
	if err != nil {
		return gen.CreateBackup500JSONResponse{ServerErrorJSONResponse: gen.ServerErrorJSONResponse{
			Code: "backup_failed", Message: "Não foi possível gravar o backup. Veja a pasta de backups e os logs.",
		}}, nil
	}
	a.d.Log.Info("manual backup", "user", auth.UserID(ctx), "file", info.Name)
	return gen.CreateBackup201JSONResponse(toBackup(info)), nil
}

// ExportData returns every table as CSV inside a zip file (RNF-18).
func (a *API) ExportData(ctx context.Context, _ gen.ExportDataRequestObject) (gen.ExportDataResponseObject, error) {
	var buf bytes.Buffer
	if err := export.WriteZip(ctx, a.d.DB, &buf); err != nil {
		return nil, err
	}
	name := fmt.Sprintf(`attachment; filename="radar-%s.zip"`, time.Now().In(a.d.Config.Loc).Format("20060102-1504"))
	return gen.ExportData200ApplicationzipResponse{
		Body:          &buf,
		ContentLength: int64(buf.Len()),
		Headers:       gen.ExportData200ResponseHeaders{ContentDisposition: &name},
	}, nil
}

// toMe converts an account user to the API model.
func toMe(u account.User) gen.Me {
	return gen.Me{Id: u.ID, Login: u.Login, Name: u.Name, Theme: gen.Theme(u.Theme), Haptics: u.Haptics}
}

// toBackup converts a backup copy to the API model.
func toBackup(b backup.Info) gen.Backup {
	return gen.Backup{Name: b.Name, CreatedAt: b.CreatedAt, SizeBytes: b.SizeBytes, Kind: gen.BackupKind(b.Kind)}
}

// unauthenticated is the error body for a missing or stale session.
func unauthenticated() gen.UnauthorizedJSONResponse {
	return gen.UnauthorizedJSONResponse{Code: "unauthenticated", Message: "Sua sessão expirou. Entre novamente."}
}

// dbSize returns the size of the database file plus its WAL, in bytes.
func dbSize(path string) int64 {
	var total int64
	for _, p := range []string{path, path + "-wal"} {
		if st, err := os.Stat(p); err == nil {
			total += st.Size()
		}
	}
	return total
}
