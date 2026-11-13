package server

import (
	"context"

	"github.com/italogfx/radar-finance/internal/account"
	"github.com/italogfx/radar-finance/internal/api/gen"
	"github.com/italogfx/radar-finance/internal/auth"
	"github.com/italogfx/radar-finance/internal/problem"
	"github.com/italogfx/radar-finance/internal/scope"
)

// errAdminOnly refuses group management to someone who is not an administrator of the active group (RF-80).
var errAdminOnly = problem.Forbidden("group_admin_only", "Só um administrador do grupo pode fazer isso.")

// GetGroup returns the active group with its members (RF-79).
func (a *API) GetGroup(ctx context.Context, _ gen.GetGroupRequestObject) (gen.GetGroupResponseObject, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return nil, err
	}
	g, err := a.groupDetail(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return gen.GetGroup200JSONResponse(g), nil
}

// RenameGroup renames the active group; administrators only (RF-80).
func (a *API) RenameGroup(ctx context.Context, req gen.RenameGroupRequestObject) (gen.RenameGroupResponseObject, error) {
	groupID, err := a.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := a.d.Accounts.RenameGroup(ctx, groupID, req.Body.Name); err != nil {
		return nil, err
	}
	g, err := a.groupDetail(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return gen.RenameGroup200JSONResponse(g), nil
}

// CreateGroup creates another group with the caller as its administrator (RF-78, RF-80).
func (a *API) CreateGroup(ctx context.Context, req gen.CreateGroupRequestObject) (gen.CreateGroupResponseObject, error) {
	if _, err := a.requireAdmin(ctx); err != nil {
		return nil, err
	}
	g, err := a.d.Accounts.CreateGroup(ctx, req.Body.Name, auth.UserID(ctx))
	if err != nil {
		return nil, err
	}
	a.d.Log.Info("group created", "group", g.ID, "user", auth.UserID(ctx))
	return gen.CreateGroup201JSONResponse{Id: g.ID, Name: g.Name, Role: gen.GroupRole(g.Role)}, nil
}

// AddGroupMember adds a person already registered to the active group with the PIN of that person as consent (RF-80).
func (a *API) AddGroupMember(ctx context.Context, req gen.AddGroupMemberRequestObject) (gen.AddGroupMemberResponseObject, error) {
	groupID, err := a.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	// The person types the own PIN as consent, so no administrator brings in the login of someone else (RF-80).
	if _, err := a.d.Accounts.JoinWithPIN(ctx, groupID, req.Body.Login, req.Body.Pin, account.Role(req.Body.Role)); err != nil {
		return nil, err
	}
	g, err := a.groupDetail(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return gen.AddGroupMember200JSONResponse(g), nil
}

// CreateGroupPerson registers a new person with login and PIN straight into the active group (RF-80, RN-27).
func (a *API) CreateGroupPerson(ctx context.Context, req gen.CreateGroupPersonRequestObject) (gen.CreateGroupPersonResponseObject, error) {
	groupID, err := a.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	b := req.Body
	if _, err := a.d.Accounts.CreateMember(ctx, groupID, b.Login, b.Name, b.Pin, account.Role(b.Role)); err != nil {
		return nil, err
	}
	g, err := a.groupDetail(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return gen.CreateGroupPerson200JSONResponse(g), nil
}

// SetGroupMemberRole changes the role of a member of the active group (RF-80).
func (a *API) SetGroupMemberRole(ctx context.Context, req gen.SetGroupMemberRoleRequestObject) (gen.SetGroupMemberRoleResponseObject, error) {
	groupID, err := a.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err := a.d.Accounts.SetRole(ctx, groupID, req.Id, account.Role(req.Body.Role)); err != nil {
		return nil, err
	}
	g, err := a.groupDetail(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return gen.SetGroupMemberRole200JSONResponse(g), nil
}

// ResetGroupMemberPin sets a new PIN for a member of the active group and lifts any lock (RF-80, RN-27).
func (a *API) ResetGroupMemberPin(ctx context.Context, req gen.ResetGroupMemberPinRequestObject) (gen.ResetGroupMemberPinResponseObject, error) {
	groupID, err := a.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	// Only people of this group, and only when the caller runs every group of theirs: a PIN opens all of them (RF-80).
	if _, ok, err := a.d.Accounts.Membership(ctx, groupID, req.Id); err != nil {
		return nil, err
	} else if !ok {
		return nil, account.ErrNotMember
	}
	if err := a.d.Accounts.CanResetPIN(ctx, auth.UserID(ctx), req.Id); err != nil {
		return nil, err
	}
	if err := a.d.Accounts.SetPIN(ctx, req.Id, req.Body.Pin); err != nil {
		return nil, err
	}
	a.d.Log.Info("pin reset", "group", groupID, "user", req.Id, "by", auth.UserID(ctx))
	return gen.ResetGroupMemberPin204Response{}, nil
}

// requireAdmin returns the active group when the caller administers it, or errAdminOnly (RF-80).
func (a *API) requireAdmin(ctx context.Context) (int64, error) {
	groupID, err := scope.Group(ctx)
	if err != nil {
		return 0, err
	}
	role, ok, err := a.d.Accounts.Membership(ctx, groupID, auth.UserID(ctx))
	if err != nil {
		return 0, err
	}
	if !ok || role != account.RoleAdmin {
		return 0, errAdminOnly
	}
	// Everyone runs her own personal space, which must not open the management of groups (RF-80, RF-83).
	if g, err := a.d.Accounts.GetGroup(ctx, groupID); err != nil {
		return 0, err
	} else if g.Personal {
		return 0, errAdminOnly
	}
	return groupID, nil
}

// groupDetail reads a group with its members for the API.
func (a *API) groupDetail(ctx context.Context, groupID int64) (gen.GroupDetail, error) {
	g, err := a.d.Accounts.GetGroup(ctx, groupID)
	if err != nil {
		return gen.GroupDetail{}, err
	}
	members, err := a.d.Accounts.Members(ctx, groupID)
	if err != nil {
		return gen.GroupDetail{}, err
	}
	out := gen.GroupDetail{Id: g.ID, Name: g.Name, Members: make([]gen.GroupMember, 0, len(members))}
	for _, m := range members {
		out.Members = append(out.Members, gen.GroupMember{
			UserId: m.UserID, PersonId: m.PersonID, Login: m.Login, Name: m.Name, Role: gen.GroupRole(m.Role), WeightBp: int(m.WeightBP), HasPin: m.HasPIN,
		})
	}
	return out, nil
}

// SetGroupWeights sets the weight of every member in the settlement, in force from today; administrators only (RF-79, RN-28).
func (a *API) SetGroupWeights(ctx context.Context, req gen.SetGroupWeightsRequestObject) (gen.SetGroupWeightsResponseObject, error) {
	if req.Body == nil {
		return nil, errNoBody
	}
	groupID, err := a.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	members, err := a.d.Accounts.Members(ctx, groupID)
	if err != nil {
		return nil, err
	}
	person := map[int64]int64{}
	for _, m := range members {
		person[m.UserID] = m.PersonID
	}
	weights := map[int64]int64{}
	for _, w := range req.Body.Weights {
		p, ok := person[w.UserId]
		if !ok {
			return nil, account.ErrWeights
		}
		weights[p] = int64(w.WeightBp)
	}
	if err := a.d.Accounts.SetWeights(ctx, groupID, weights); err != nil {
		return nil, err
	}
	g, err := a.groupDetail(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return gen.SetGroupWeights200JSONResponse(g), nil
}
