package secured

import (
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/trx"
	model "kasper/src/core/module/actor/model/base"
)

type Guard struct {
	IsUser    bool `json:"isUser"`
	IsInSpace bool `json:"isInSpace"`
	IsInTopic bool `json:"isInTopic"`
}

func (g *Guard) CheckValidity(app core.ICore, packet []byte, signature string, userId string, pointId string, insider ...bool) (bool, *model.Info) {
	if !g.IsUser {
		return true, model.NewInfo("", "")
	}
	if len(insider) > 0 && insider[0] && (signature == "#appletsign") {
		typ := ""
		app.ModifyState(true, func(trx trx.ITrx) error {
			typ = string(trx.GetColumn("User", userId, "type"))
			return nil
		})
		if typ == "machine" {
			if !g.IsInSpace {
				return true, model.NewGodInfo(userId, "", false)
			}
			hasAccess := app.Tools().Security().HasAccessToPoint(userId, pointId)
			if !hasAccess {
				return false, &model.Info{}
			}
			return true, model.NewGodInfo(userId, pointId, false)
		}
	}
	identified, _, isGod := app.Tools().Security().AuthWithSignature(userId, packet, signature)
	if !identified {
		return false, &model.Info{}
	}
	if !g.IsInSpace {
		return true, model.NewGodInfo(userId, "", isGod)
	}
	hasAccess := app.Tools().Security().HasAccessToPoint(userId, pointId)
	if !hasAccess {
		return false, &model.Info{}
	}
	return true, model.NewGodInfo(userId, pointId, isGod)
}

func (g *Guard) CheckValidityForChain(app core.ICore, packet []byte, signature string, userId string, pointId string) (bool, *model.Info) {
	if !g.IsUser {
		return true, model.NewInfo("", "")
	}
	if signature == "#appletsign" {
		typ := ""
		app.ModifyState(true, func(trx trx.ITrx) error {
			typ = string(trx.GetColumn("User", userId, "type"))
			return nil
		})
		if typ == "machine" {
			if !g.IsInSpace {
				return true, model.NewGodInfo(userId, "", false)
			}
			hasAccess := app.Tools().Security().HasAccessToPoint(userId, pointId)
			if !hasAccess {
				return false, &model.Info{}
			}
			return true, model.NewGodInfo(userId, pointId, false)
		}
	}
	identified, _, isGod := app.Tools().Security().AuthWithSignature(userId, packet, signature)
	if !identified {
		return false, &model.Info{}
	}
	if !g.IsInSpace {
		return true, model.NewGodInfo(userId, "", isGod)
	}
	hasAccess := app.Tools().Security().HasAccessToPoint(userId, pointId)
	if !hasAccess {
		return false, &model.Info{}
	}
	return true, model.NewGodInfo(userId, pointId, isGod)
}

func (g *Guard) CheckIdentity(app core.ICore, packet []byte, signature string, userId string) bool {
	if !g.IsUser {
		return true
	}
	identified, _, _ := app.Tools().Security().AuthWithSignature(userId, packet, signature)
	return identified
}
