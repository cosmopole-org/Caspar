package actions_invite

import (
	"errors"
	"kasper/src/abstract/models/action"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/state"
	inputsinvites "kasper/src/shell/api/inputs/invites"
	"kasper/src/shell/api/model"
	outputsinvites "kasper/src/shell/api/outputs/invites"
	updatesinvites "kasper/src/shell/api/updates/invites"
	updates_points "kasper/src/shell/api/updates/points"
	"kasper/src/shell/utils/future"
	"log"
	"strconv"
	"time"
)

type Actions struct {
	App           core.ICore
	modelExtender map[string]map[string]action.ExtendedField
}

func Install(a *Actions, params ...any) error {
	if len(params) >= 1 {
		a.modelExtender = params[0].(map[string]map[string]action.ExtendedField)
	} else {
		a.modelExtender = map[string]map[string]action.ExtendedField{}
	}
	if _, ok := a.modelExtender["user"]; !ok {
		a.modelExtender["user"] = map[string]action.ExtendedField{}
	}
	return nil
}

// Create /invites/create check [ true true false ] access [ true false false false POST ]
func (a *Actions) Create(state state.IState, input inputsinvites.CreateInput) (any, error) {
	trx := state.Trx()
	if trx.GetLink("admin::"+state.Info().PointId()+"::"+state.Info().UserId()) != "true" {
		return nil, errors.New("you are not admin")
	}
	if !trx.HasObj("Point", state.Info().PointId()) {
		return nil, errors.New("point not found")
	}
	point := model.Point{Id: state.Info().PointId()}.Pull(trx)
	if point.Tag == "home" {
		return nil, errors.New("home is not extendable")
	}
	trx.PutLink("invite::"+point.Id+"::"+input.UserId, "true")
	trx.PutLink("inviteto::"+input.UserId+"::"+point.Id, "true")
	trx.PutLink("invitetime::"+point.Id+"::"+input.UserId, strconv.FormatInt(time.Now().UnixMilli(), 10))
	future.Async(func() {
		a.App.Tools().Signaler().SignalUser("invites/create", input.UserId, updatesinvites.Create{Point: point}, true)
	}, false)
	return outputsinvites.CreateOutput{}, nil
}

// ListPointInvites /invites/listPointInvites check [ true true false ] access [ true false false false POST ]
func (a *Actions) ListPointInvites(state state.IState, input inputsinvites.ListPointInvitesInput) (any, error) {
	trx := state.Trx()
	if trx.GetLink("admin::"+state.Info().PointId()+"::"+state.Info().UserId()) != "true" {
		return nil, errors.New("you are not admin")
	}
	if !trx.HasObj("Point", state.Info().PointId()) {
		return nil, errors.New("point not found")
	}
	prefix := "invite::" + state.Info().PointId() + "::"
	links, err := trx.GetLinksList(prefix, 0, 1000)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	users := []map[string]any{}
	for _, link := range links {
		user := model.User{Id: link[len(prefix):]}.Pull(trx)
		t, _ := strconv.ParseInt(trx.GetLink("invitetime::"+state.Info().PointId()+"::"+user.Id), 10, 64)
		result := map[string]any{
			"id":       user.Id,
			"username": user.Username,
			"type":     user.Typ,
			"time":     t,
		}
		for _, v := range a.modelExtender["user"] {
			if meta, err := trx.GetJson("UserMeta::"+user.Id, v.Path); err == nil || meta[v.Name] != nil {
				result[v.Name] = meta[v.Name]
			}
		}
		users = append(users, result)
	}
	return map[string]any{"users": users}, nil
}

// ListUserInvites /invites/listUserInvites check [ true false false ] access [ true false false false POST ]
func (a *Actions) ListUserInvites(state state.IState, input inputsinvites.ListUserInvitesInput) (any, error) {
	trx := state.Trx()
	prefix := "inviteto::" + state.Info().UserId() + "::"
	links, err := trx.GetLinksList(prefix, 0, 1000)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	points := []map[string]any{}
	for _, link := range links {
		point := model.Point{Id: link[len(prefix):]}.Pull(trx)
		t, _ := strconv.ParseInt(trx.GetLink("invitetime::"+point.Id+"::"+state.Info().UserId()), 10, 64)

		result := map[string]any{
			"id":          point.Id,
			"isPublic":    point.IsPublic,
			"persHist":    point.PersHist,
			"memberCount": point.MemberCount,
			"signalCount": point.SignalCount,
			"parentId":    point.ParentId,
			"tag":         point.Tag,
			"time":        t,
		}
		for _, v := range a.modelExtender["point"] {
			if meta, err := trx.GetJson("PointMeta::"+point.Id, v.Path); err == nil || meta[v.Name] != nil {
				result[v.Name] = meta[v.Name]
			}
		}
		points = append(points, result)
	}
	return map[string]any{"points": points}, nil
}

// Cancel /invites/cancel check [ true true false ] access [ true false false false POST ]
func (a *Actions) Cancel(state state.IState, input inputsinvites.CancelInput) (any, error) {
	trx := state.Trx()
	if trx.GetLink("admin::"+state.Info().PointId()+"::"+state.Info().UserId()) != "true" {
		return nil, errors.New("you are not admin")
	}
	if trx.GetLink("invite::"+state.Info().PointId()+"::"+input.UserId) != "true" {
		return nil, errors.New("invitation does not exist")
	}
	trx.DelKey("link::invite::" + state.Info().PointId() + "::" + input.UserId)
	trx.DelKey("link::inviteto::" + input.UserId + "::" + state.Info().PointId())
	trx.DelKey("link::invitetime::" + state.Info().PointId() + "::" + input.UserId)
	point := model.Point{Id: state.Info().PointId()}.Pull(trx)
	future.Async(func() {
		a.App.Tools().Signaler().SignalUser("invites/cancel", input.UserId, updatesinvites.Cancel{Point: point}, true)
	}, false)
	return outputsinvites.CancelOutput{}, nil
}

// Accept /invites/accept check [ true false false ] access [ true false false false POST ]
func (a *Actions) Accept(state state.IState, input inputsinvites.AcceptInput) (any, error) {
	trx := state.Trx()
	if trx.GetLink("invite::"+input.PointId+"::"+state.Info().UserId()) != "true" {
		return nil, errors.New("invitation does not exist")
	}
	trx.DelKey("link::invite::" + input.PointId + "::" + state.Info().UserId())
	trx.DelKey("link::inviteto::" + state.Info().UserId() + "::" + input.PointId)
	trx.DelKey("link::invitetime::" + input.PointId + "::" + state.Info().UserId())
	trx.PutLink("member::"+input.PointId+"::"+state.Info().UserId(), "true")
	trx.PutLink("memberof::"+state.Info().UserId()+"::"+input.PointId, "true")
	a.App.Tools().Signaler().JoinGroup(input.PointId, state.Info().UserId())
	admins, err := trx.GetLinksList("admin::"+input.PointId+"::", -1, -1)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	user := model.User{Id: state.Info().UserId()}.Pull(trx)
	for _, admin := range admins {
		future.Async(func() {
			a.App.Tools().Signaler().SignalUser("invites/accept", admin, updatesinvites.Accept{User: user, PointId: input.PointId}, true)
		}, false)
	}
	future.Async(func() {
		a.App.Tools().Signaler().SignalGroup("spaces/userJoined", input.PointId, updates_points.Join{PointId: input.PointId, User: user}, true, []string{})
	}, false)
	return outputsinvites.AcceptOutput{}, nil
}

// Decline /invites/decline check [ true false false ] access [ true false false false POST ]
func (a *Actions) Decline(state state.IState, input inputsinvites.DeclineInput) (any, error) {
	trx := state.Trx()
	if trx.GetLink("invite::"+input.PointId+"::"+state.Info().UserId()) != "true" {
		return nil, errors.New("invitation does not exist")
	}
	trx.DelKey("link::invite::" + input.PointId + "::" + state.Info().UserId())
	trx.DelKey("link::inviteto::" + state.Info().UserId() + "::" + input.PointId)
	trx.DelKey("link::invitetime::" + input.PointId + "::" + state.Info().UserId())
	admins, err := trx.GetLinksList("admin::"+input.PointId+"::", -1, -1)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	user := model.User{Id: state.Info().UserId()}.Pull(trx)
	for _, admin := range admins {
		future.Async(func() {
			a.App.Tools().Signaler().SignalUser("invites/decline", admin, updatesinvites.Accept{User: user, PointId: input.PointId}, true)
		}, false)
	}
	return outputsinvites.DeclineOutput{}, nil
}
