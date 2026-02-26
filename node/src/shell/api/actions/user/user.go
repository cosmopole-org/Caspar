package actions_user

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"kasper/src/abstract/models/action"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/trx"
	"kasper/src/abstract/state"
	"kasper/src/core/module/actor/model/base"
	mainstate "kasper/src/core/module/actor/model/state"
	inputsusers "kasper/src/shell/api/inputs/users"
	models "kasper/src/shell/api/model"
	outputsusers "kasper/src/shell/api/outputs/users"
	"kasper/src/shell/utils/crypto"
	"log"
	"strings"

	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

type Actions struct {
	App           core.ICore
	OauthCtx      context.Context
	firebaseApp   *firebase.App
	modelExtender map[string]map[string]action.ExtendedField
}

func (a *Actions) initFirebase() {
	opt := option.WithCredentialsFile("/app/serviceAccounts.json")
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		log.Fatalf("Error initializing Firebase app: %v\n", err)
	}
	a.firebaseApp = app
}

func Install(a *Actions, params ...any) error {
	a.OauthCtx = context.Background()
	a.initFirebase()
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

// Authenticate /users/authenticate check [ true false false ] access [ true false false false POST ]
func (a *Actions) Authenticate(state state.IState, _ inputsusers.AuthenticateInput) (any, error) {
	_, res, _ := a.App.Actor().FetchAction("/users/get").Act(mainstate.NewState(base.NewInfo("", ""), state.Trx()), inputsusers.GetInput{UserId: state.Info().UserId()})
	return outputsusers.AuthenticateOutput{Authenticated: true, User: res.(outputsusers.GetOutput).User}, nil
}

// Transfer /users/transfer check [ true false false ] access [ true false false false POST ]
func (a *Actions) Transfer(state state.IState, input inputsusers.TransferInput) (any, error) {
	user := models.User{Id: state.Info().UserId()}.Pull(state.Trx())
	if user.Balance < input.Amount {
		return nil, errors.New("your balance is not enough")
	}
	toUserId := state.Trx().GetIndex("User", "username", "id", input.ToUsername)
	if toUserId == "" {
		return nil, errors.New("target user not found")
	}
	toUser := models.User{Id: toUserId}.Pull(state.Trx())
	user.Balance -= input.Amount
	toUser.Balance += input.Amount
	user.Push(state.Trx())
	toUser.Push(state.Trx())
	return map[string]any{}, nil
}

// Mint /users/mint check [ true false false ] access [ true false false false POST ]
func (a *Actions) Mint(state state.IState, input inputsusers.MintInput) (any, error) {
	if state.Info().UserId() != "1@global" {
		return nil, errors.New("access denied")
	}
	username := models.User{Id: state.Trx().GetLink("UserEmailToId::" + input.ToUserEmail)}.Pull(state.Trx()).Username
	toUserId := state.Trx().GetIndex("User", "username", "id", username)
	if toUserId == "" {
		return nil, errors.New("target user not found")
	}
	toUser := models.User{Id: toUserId}.Pull(state.Trx())
	toUser.Balance += input.Amount
	toUser.Push(state.Trx())
	return map[string]any{}, nil
}

// CheckSign /users/checkSign check [ true false false ] access [ true false false false POST ]
func (a *Actions) CheckSign(state state.IState, input inputsusers.CheckSignInput) (any, error) {
	if state.Info().UserId() != "1@global" {
		return nil, errors.New("access denied")
	}
	data, err := base64.StdEncoding.DecodeString(input.Payload)
	if err != nil {
		log.Println(err)
		return map[string]any{"valid": false}, nil
	}
	if success, _, _ := a.App.Tools().Security().AuthWithSignature(input.UserId, data, input.Signature); success {
		email := state.Trx().GetLink("UserIdToEmail::" + input.UserId)
		return map[string]any{"valid": true, "email": email}, nil
	} else {
		return map[string]any{"valid": false}, nil
	}
}

// LockToken /users/lockToken check [ true false false ] access [ true false false false POST ]
func (a *Actions) LockToken(state state.IState, input inputsusers.LockTokenInput) (any, error) {
	user := models.User{Id: state.Info().UserId()}.Pull(state.Trx())
	if user.Balance < input.Amount {
		return nil, errors.New("your balance is not enough")
	}
	lockId := crypto.SecureUniqueString()
	if input.Type == "exec" {
		validators := a.App.Tools().Network().Chain().GetValidatorsOfMachineShard(input.Target)
		str, err := json.Marshal(validators)
		if err != nil {
			return nil, err
		}
		if input.Amount < int64(len(validators)) {
			return nil, errors.New("amount to be locked can not be less than validators count")
		}
		user.Balance -= input.Amount
		user.Push(state.Trx())
		state.Trx().PutJson("Json::User::"+state.Info().UserId(), "lockedTokens."+lockId, map[string]any{"type": "exec", "amount": input.Amount, "validators": string(str)}, true)
	} else if input.Type == "pay" {
		if !state.Trx().HasObj("User", input.Target) {
			return nil, errors.New("target user not acceptable")
		}
		user.Balance -= input.Amount
		user.Push(state.Trx())
		state.Trx().PutJson("Json::User::"+state.Info().UserId(), "lockedTokens."+lockId, map[string]any{"type": "pay", "amount": input.Amount, "userId": input.Target}, true)
	} else {
		return nil, errors.New("unknown lock type")
	}
	return map[string]any{"tokenId": lockId}, nil
}

// ConsumeLock /users/consumeLock check [ true false false ] access [ true false false false POST ]
func (a *Actions) ConsumeLock(state state.IState, input inputsusers.ConsumeLockInput) (any, error) {
	receiver := models.User{Id: state.Info().UserId()}.Pull(state.Trx())
	if input.Type == "pay" {
		if !state.Trx().HasObj("User", input.UserId) {
			return nil, errors.New("payer user not found")
		}
		if success, _, _ := a.App.Tools().Security().AuthWithSignature(input.UserId, []byte(input.LockId), input.Signature); success {
			sender := models.User{Id: input.UserId}.Pull(state.Trx())
			if payment, err := state.Trx().GetJson("Json::User::"+sender.Id, "lockedTokens."+input.LockId); err == nil {
				if typ, ok := payment["type"].(string); ok && (typ == "pay") {
					if amount, ok := payment["amount"].(float64); ok && (int64(amount) == input.Amount) {
						if target, ok := payment["userId"].(string); ok && (target == receiver.Id) {
							sender.Balance -= input.Amount
							sender.Push(state.Trx())
							receiver.Balance += input.Amount
							receiver.Push(state.Trx())
							state.Trx().DelJson("Json::User::"+sender.Id, "lockedTokens."+input.LockId)
							return map[string]any{"success": true}, nil
						} else {
							return nil, errors.New("you are not target")
						}
					} else {
						return nil, errors.New("amount of payment not matched")
					}
				} else {
					return nil, errors.New("type is not payment")
				}
			} else {
				return nil, errors.New("lock not found")
			}
		} else {
			return nil, errors.New("signature not verified")
		}
	} else {
		return nil, errors.New("unknown lock type")
	}
}

// Login /users/login check [ false false false ] access [ true false false false POST ]
func (a *Actions) Login(state state.IState, input inputsusers.LoginInput) (any, error) {

	ctx := a.OauthCtx
	client, err := a.firebaseApp.Auth(ctx)
	if err != nil {
		log.Println(err)
		e := errors.New("error getting Auth client")
		log.Println(e)
		return nil, e
	}
	token, err := client.VerifyIDToken(ctx, input.EmailToken)
	if err != nil {
		log.Println(err)
		e := errors.New("invalid ID token")
		log.Println(e)
		return nil, e
	}
	email, ok := token.Claims["email"].(string)
	if !ok {
		e := errors.New("email claim not found or invalid")
		log.Println(e)
		return nil, e
	}

	trx := state.Trx()
	userId := trx.GetLink("UserEmailToId::" + email)
	log.Println("fetching email:", "["+email+"]", "["+userId+"]")
	if userId != "" {
		user := models.User{Id: userId}.Pull(trx)
		session := models.Session{Id: trx.GetIndex("Session", "userId", "id", user.Id)}.Pull(trx)
		privKey := trx.GetLink("UserPrivateKey::" + user.Id)
		return outputsusers.LoginOutput{User: user, Session: session, PrivateKey: privKey}, nil
	}
	if !trx.HasIndex("User", "username", "id", input.Username+"@"+a.App.Id()) {
		priKeyRaw, pubKeyRaw := crypto.SecureKeyPairs("")
		priKey := string(priKeyRaw)
		pubKey := string(pubKeyRaw)
		req := inputsusers.CreateInput{
			Username:  input.Username,
			PublicKey: pubKey,
			Metadata:  input.Metadata,
		}
		bin, _ := json.Marshal(req)
		sign := a.App.SignPacket(bin)
		_, res, err2 := a.App.Actor().FetchAction("/users/create").(action.ISecureAction).SecurelyAct("", "", bin, sign, req, a.App.Id())
		if err2 != nil {
			return nil, err2
		}
		var response outputsusers.CreateOutput
		b, e := json.Marshal(res)
		if e != nil {
			log.Println(e)
			return nil, e
		}
		e = json.Unmarshal(b, &response)
		if e != nil {
			log.Println(e)
			return nil, e
		}
		trx.PutLink("UserPrivateKey::"+response.User.Id, priKey)
		trx.PutLink("UserEmailToId::"+email, response.User.Id)
		trx.PutLink("UserIdToEmail::"+response.User.Id, email)
		log.Println("saving email:", "["+email+"]", "["+response.User.Id+"]")
		return outputsusers.LoginOutput{User: response.User, Session: response.Session, PrivateKey: priKey}, nil
	} else {
		return nil, errors.New("username already exist")
	}
}

// Create /users/create check [ false false false ] access [ true false false false POST ]
func (a *Actions) Create(state state.IState, input inputsusers.CreateInput) (any, error) {
	var (
		user    models.User
		session models.Session
	)
	trx := state.Trx()
	if trx.HasIndex("User", "username", "id", input.Username+"@"+state.Source()) {
		return nil, errors.New("username already exists")
	}
	user = models.User{Id: a.App.Tools().Storage().GenId(trx, input.Origin()), Typ: "human", Balance: 1000000000000000, PublicKey: input.PublicKey, Username: input.Username + "@" + state.Source()}
	session = models.Session{Id: a.App.Tools().Storage().GenId(trx, input.Origin()), UserId: user.Id}
	user.Push(trx)
	session.Push(trx)

	for _, v := range a.modelExtender["user"] {
		trx.PutJson("UserMeta::"+user.Id, v.Path, map[string]any{
			v.Name: v.Default,
		}, true)
	}
	trx.PutJson("UserMeta::"+user.Id, "metadata", input.Metadata, true)
	for _, v := range a.modelExtender["user"] {
		if meta, err := trx.GetJson("UserMeta::"+user.Id, v.Path); err != nil || meta[v.Name] == nil {
			if v.Required {
				return nil, errors.New(v.Name + " can not be empty.")
			}
		} else {
			if v.Searchable {
				trx.PutIndex("User", v.Name, "id", user.Id+"->"+meta[v.Name].(string), []byte(user.Id))
			}
		}
	}
	err := trx.PutJson("UserMeta::"+user.Id, "metadata.private.settings.privacy", map[string]any{"onlineView": "all"}, true)
	if err != nil {
		log.Println(err)
	}

	point := models.Point{Id: a.App.Tools().Storage().GenId(trx, "global"), Tag: "home", IsPublic: false, PersHist: true, ParentId: ""}
	point.Push(trx)
	trx.PutLink("memberof::"+user.Id+"::"+point.Id, "true")
	trx.PutLink("member::"+point.Id+"::"+user.Id, "true")
	trx.PutLink("admin::"+point.Id+"::"+user.Id, "true")
	trx.PutLink("adminof::"+user.Id+"::"+point.Id, "true")
	trx.PutJson("PointMeta::"+point.Id, "metadata", map[string]any{"public": map[string]any{"profile": map[string]any{"title": "Home", "avatar": "avatar"}}}, false)
	trx.PutIndex("Point", "title", "id", point.Id+"->home", []byte(point.Id))
	a.App.Tools().Signaler().JoinGroup(point.Id, user.Id)
	return outputsusers.CreateOutput{User: user, Session: session}, nil
}

// Delete /users/delete check [ true false false ] access [ true false false false POST ]
func (a *Actions) Delete(state state.IState, input inputsusers.DeleteInput) (any, error) {
	trx := state.Trx()

	ctx := a.OauthCtx
	client, err := a.firebaseApp.Auth(ctx)
	if err != nil {
		log.Println(err)
		e := errors.New("error getting Auth client")
		log.Println(e)
		return nil, e
	}
	user := models.User{Id: state.Info().UserId()}.Pull(trx)
	email := trx.GetLink("UserIdToEmail::" + user.Id)
	userRecord, err := client.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, errors.New("error getting user by email " + email + " " + err.Error())
	}
	err = client.DeleteUser(ctx, userRecord.UID)
	if err != nil {
		return nil, errors.New("error deleting user: " + err.Error())
	}
	for _, v := range a.modelExtender["user"] {
		if v.Searchable {
			if meta, err := trx.GetJson("UserMeta::"+user.Id, v.Path); err == nil || meta[v.Name] != nil {
				trx.DelIndex("User", v.Name, "id", user.Id+"->"+meta[v.Name].(string))
			}
		}
	}
	trx.DelKey("link::UserPrivateKey::" + user.Id)
	trx.DelKey("link::UserEmailToId::" + email)
	trx.DelKey("link::UserIdToEmail::" + user.Id)
	trx.DelIndex("User", "username", "id", user.Username)
	user.Username = "deleted_user@deleted"
	user.PublicKey = ""
	user.Balance = 0
	user.Push(trx)
	trx.DelJson("UserMeta::"+user.Id, "metadata")
	return map[string]any{}, nil
}

// Update /users/update check [ true false false ] access [ true false false false POST ]
func (a *Actions) Update(state state.IState, input inputsusers.UpdateInput) (any, error) {
	trx := state.Trx()
	for _, v := range a.modelExtender["user"] {
		if v.Searchable {
			if meta, err := trx.GetJson("UserMeta::"+state.Info().UserId(), v.Path); err == nil || meta[v.Name] != nil {
				trx.DelIndex("User", v.Name, "id", state.Info().UserId()+"->"+meta[v.Name].(string))
			}
		}
	}
	trx.PutJson("UserMeta::"+state.Info().UserId(), "metadata", input.Metadata, true)
	for _, v := range a.modelExtender["user"] {
		if meta, err := trx.GetJson("UserMeta::"+state.Info().UserId(), v.Path); err != nil || meta[v.Name] == nil {
			if v.Required {
				return nil, errors.New(v.Name + " can not be empty.")
			}
		} else {
			if v.Searchable {
				trx.PutIndex("User", v.Name, "id", state.Info().UserId()+"->"+meta[v.Name].(string), []byte(state.Info().UserId()))
			}
		}
	}
	return map[string]any{}, nil
}

// Meta /users/meta check [ true false false ] access [ true false false false GET ]
func (a *Actions) Meta(state state.IState, input inputsusers.MetaInput) (any, error) {
	trx := state.Trx()
	if !trx.HasObj("User", input.UserId) {
		return nil, errors.New("user not found")
	}
	if (state.Info().UserId() != input.UserId) && strings.HasPrefix(input.Path, "private.") {
		return nil, errors.New("access denied")
	}
	metadata, err := trx.GetJson("UserMeta::"+input.UserId, "metadata."+input.Path)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return map[string]any{"metadata": metadata}, nil
}

func (a *Actions) getUserStatus(trx trx.ITrx, targetUserId string, requesterUserId string) string {
	if targetUserId == requesterUserId {
		if a.App.Tools().Signaler().Listeners().Has(targetUserId) {
			return "online"
		} else {
			return "offline"
		}
	} else {
		metaPriv, err := trx.GetJson("UserMeta::"+targetUserId, "metadata.private.settings.privacy")
		if err != nil {
			log.Println(err)
			return "last seen recently"
		}
		onlineView := metaPriv["onlineView"].(string)
		if onlineView == "all" {
			if a.App.Tools().Signaler().Listeners().Has(targetUserId) {
				return "online"
			} else {
				return "offline"
			}
		} else if onlineView == "contacts" {
			if metaCont, _ := trx.GetJson("UserMeta::"+targetUserId, "metadata.private.contacts"); metaCont[requesterUserId] == true {
				if a.App.Tools().Signaler().Listeners().Has(targetUserId) {
					return "online"
				} else {
					return "offline"
				}
			} else {
				return "last seen recently"
			}
		} else {
			return "last seen recently"
		}
	}
}

// Get /users/get check [ true false false ] access [ true false false false GET ]
func (a *Actions) Get(state state.IState, input inputsusers.GetInput) (any, error) {
	trx := state.Trx()
	if !trx.HasObj("User", input.UserId) {
		return nil, errors.New("user not found")
	}
	user := models.User{Id: input.UserId}.Pull(trx)
	result := map[string]any{
		"id":        user.Id,
		"publicKey": user.PublicKey,
		"type":      user.Typ,
		"username":  user.Username,
	}
	for _, v := range a.modelExtender["user"] {
		if meta, err := trx.GetJson("UserMeta::"+user.Id, v.Path); err == nil || meta[v.Name] != nil {
			result[v.Name] = meta[v.Name]
		}
	}
	if input.UserId == state.Info().UserId() {
		result["balance"] = user.Balance
	}

	result["status"] = a.getUserStatus(trx, input.UserId, state.Info().UserId())

	return outputsusers.GetOutput{User: result}, nil
}

// GetByUsername /users/getByUsername check [ true false false ] access [ true false false false GET ]
func (a *Actions) GetByUsername(state state.IState, input inputsusers.GetByUsernameInput) (any, error) {
	trx := state.Trx()
	userId := trx.GetIndex("User", "username", "id", input.Username)
	user := models.User{Id: userId}.Pull(trx)
	result := map[string]any{
		"id":        user.Id,
		"publicKey": user.PublicKey,
		"type":      user.Typ,
		"username":  user.Username,
	}
	for _, v := range a.modelExtender["user"] {
		if meta, err := trx.GetJson("UserMeta::"+user.Id, v.Path); err == nil || meta[v.Name] != nil {
			result[v.Name] = meta[v.Name]
		}
	}
	if userId == state.Info().UserId() {
		result["balance"] = user.Balance
	}

	result["status"] = a.getUserStatus(trx, userId, state.Info().UserId())

	return outputsusers.GetOutput{User: result}, nil
}

// Find /users/find check [ true false false ] access [ true false false false GET ]
func (a *Actions) Find(state state.IState, input inputsusers.FindInput) (any, error) {
	trx := state.Trx()
	userId := trx.GetIndex("User", "username", "id", input.Username)
	if userId == "" {
		return nil, errors.New("user not found")
	}
	user := models.User{Id: userId}.Pull(trx)
	result := map[string]any{
		"id":        user.Id,
		"publicKey": user.PublicKey,
		"type":      user.Typ,
		"username":  user.Username,
	}
	for _, v := range a.modelExtender["user"] {
		if meta, err := trx.GetJson("UserMeta::"+user.Id, v.Path); err == nil || meta[v.Name] != nil {
			result[v.Name] = meta[v.Name]
		}
	}
	if userId == state.Info().UserId() {
		result["balance"] = user.Balance
	}
	result["status"] = a.getUserStatus(trx, user.Id, state.Info().UserId())
	return outputsusers.GetOutput{User: result}, nil
}

// List /users/list check [ true false false ] access [ true false false false GET ]
func (a *Actions) List(state state.IState, input inputsusers.ListInput) (any, error) {
	trx := state.Trx()
	if input.Param != "username" && input.Param != "name" {
		return nil, errors.New("param is not searchable")
	}
	users, err := models.User{}.Search(trx, input.Offset, input.Count, input.Param, input.Query, map[string]string{"type": "human"})
	if err != nil {
		log.Println(err)
		return nil, err
	}
	results := []map[string]any{}
	for _, user := range users {
		result := map[string]any{
			"id":        user.Id,
			"publicKey": user.PublicKey,
			"type":      user.Typ,
			"username":  user.Username,
		}
		for _, v := range a.modelExtender["user"] {
			if meta, err := trx.GetJson("UserMeta::"+user.Id, v.Path); err == nil || meta[v.Name] != nil {
				result[v.Name] = meta[v.Name]
			}
		}
		result["status"] = a.getUserStatus(trx, user.Id, state.Info().UserId())
		results = append(results, result)
	}
	return map[string]any{"users": results}, nil
}
