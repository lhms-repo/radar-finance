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
	"github.com/italogfx/radar-finance/internal/scope"
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

// ListLoginGroups lists the group names, the first step of entering (RF-01).
func (a *API) ListLoginGroups(ctx context.Context, _ gen.ListLoginGroupsRequestObject) (gen.ListLoginGroupsResponseObject, error) {
	groups, err := a.d.Accounts.Groups(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListLoginGroups200JSONResponse, 0, len(groups))
	for _, g := range groups {
		out = append(out, gen.LoginGroup{Id: g.ID, Name: g.Name})
	}
	return out, nil
}

// ListLoginMembers lists the people of the chosen group, the second step of entering (RF-01).
func (a *API) ListLoginMembers(ctx context.Context, req gen.ListLoginMembersRequestObject) (gen.ListLoginMembersResponseObject, error) {
	if g, err := a.d.Accounts.GetGroup(ctx, req.Id); err != nil {
		return nil, err
	} else if g.Personal {
		// A personal space is not a way in, and its owner is not listed by id.
		return nil, account.ErrGroupNotFound
	}
	members, err := a.d.Accounts.Members(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListLoginMembers200JSONResponse, 0, len(members))
	for _, m := range members {
		out = append(out, gen.LoginMember{UserId: m.UserID, Name: m.Name, HasPin: m.HasPIN})
	}
	return out, nil
}

// Login starts a session for a member of the group who typed the right PIN (RF-01, RN-27).
func (a *API) Login(ctx context.Context, req gen.LoginRequestObject) (gen.LoginResponseObject, error) {
	if req.Body == nil || req.Body.GroupId == 0 || req.Body.UserId == 0 {
		return gen.Login400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse{
			Code: "invalid_request", Message: "Escolha o grupo e quem está usando.",
		}}, nil
	}
	user, err := a.d.Accounts.Authenticate(ctx, req.Body.GroupId, req.Body.UserId, req.Body.Pin)
	if errors.Is(err, account.ErrNotFound) {
		return gen.Login401JSONResponse{UnauthorizedJSONResponse: gen.UnauthorizedJSONResponse{
			Code: "unknown_user", Message: "Usuário não encontrado.",
		}}, nil
	}
	if err != nil {
		if errors.Is(err, account.ErrWrongPIN) || errors.Is(err, account.ErrLocked) {
			a.d.Log.Warn("wrong pin", "user", req.Body.UserId, "group", req.Body.GroupId, "locked", errors.Is(err, account.ErrLocked))
		}
		return nil, err
	}
	if err := auth.SignIn(ctx, a.d.Sessions, user.ID, req.Body.GroupId); err != nil {
		return nil, err
	}
	a.d.Log.Info("login", "user", user.Login, "group", req.Body.GroupId)
	me, err := a.me(ctx, user, req.Body.GroupId)
	if err != nil {
		return nil, err
	}
	return gen.Login200JSONResponse(me), nil
}

// Logout destroys the session.
func (a *API) Logout(ctx context.Context, _ gen.LogoutRequestObject) (gen.LogoutResponseObject, error) {
	if err := auth.SignOut(ctx, a.d.Sessions); err != nil {
		return nil, err
	}
	return gen.Logout204Response{}, nil
}

// GetMe returns the logged in user with the active group and every group of the user (RF-81).
func (a *API) GetMe(ctx context.Context, _ gen.GetMeRequestObject) (gen.GetMeResponseObject, error) {
	user, err := a.d.Accounts.Get(ctx, auth.UserID(ctx))
	if errors.Is(err, account.ErrNotFound) {
		_ = auth.SignOut(ctx, a.d.Sessions)
		return gen.GetMe401JSONResponse{UnauthorizedJSONResponse: unauthenticated()}, nil
	}
	if err != nil {
		return nil, err
	}
	groupID, err := scope.Group(ctx)
	if err != nil {
		return nil, err
	}
	// Someone removed from the active group goes back to the entry.
	if _, ok, err := a.d.Accounts.Membership(ctx, groupID, user.ID); err != nil {
		return nil, err
	} else if !ok {
		_ = auth.SignOut(ctx, a.d.Sessions)
		return gen.GetMe401JSONResponse{UnauthorizedJSONResponse: unauthenticated()}, nil
	}
	me, err := a.me(ctx, user, groupID)
	if err != nil {
		return nil, err
	}
	return gen.GetMe200JSONResponse(me), nil
}

// SwitchGroup makes another group of the user the active one, without entering again (RF-81).
func (a *API) SwitchGroup(ctx context.Context, req gen.SwitchGroupRequestObject) (gen.SwitchGroupResponseObject, error) {
	if req.Body == nil || req.Body.GroupId == 0 {
		return gen.SwitchGroup400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse{
			Code: "invalid_request", Message: "Escolha um grupo.",
		}}, nil
	}
	user, err := a.d.Accounts.Get(ctx, auth.UserID(ctx))
	if err != nil {
		return nil, err
	}
	if _, ok, err := a.d.Accounts.Membership(ctx, req.Body.GroupId, user.ID); err != nil {
		return nil, err
	} else if !ok {
		return nil, account.ErrNotMember
	}
	if err := auth.SwitchGroup(ctx, a.d.Sessions, req.Body.GroupId); err != nil {
		return nil, err
	}
	me, err := a.me(ctx, user, req.Body.GroupId)
	if err != nil {
		return nil, err
	}
	return gen.SwitchGroup200JSONResponse(me), nil
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
	groupID, err := scope.Group(ctx)
	if err != nil {
		return nil, err
	}
	me, err := a.me(ctx, user, groupID)
	if err != nil {
		return nil, err
	}
	return gen.UpdatePreferences200JSONResponse(me), nil
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

// me converts an account user to the API model, with the active group and every group of the user (RF-81).
func (a *API) me(ctx context.Context, u account.User, groupID int64) (gen.Me, error) {
	groups, err := a.d.Accounts.GroupsOf(ctx, u.ID)
	if err != nil {
		return gen.Me{}, err
	}
	out := gen.Me{Id: u.ID, Login: u.Login, Name: u.Name, Theme: gen.Theme(u.Theme), Haptics: u.Haptics, Groups: make([]gen.GroupSummary, 0, len(groups))}
	for _, g := range groups {
		summary := gen.GroupSummary{Id: g.ID, Name: g.Name, Role: gen.GroupRole(g.Role), Personal: g.Personal}
		out.Groups = append(out.Groups, summary)
		if g.ID == groupID {
			out.Group = summary
		}
	}
	return out, nil
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
