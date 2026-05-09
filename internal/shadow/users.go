package shadow

import (
	"encoding/json"
	"fmt"
	"time"
)

type AttributeUser struct {
	Id   string             `json:"id"`
	Name string             `json:"name"`
	Role int                `json:"role"`
	Pwd  []AttributeUserPwd `json:"pwd"`
	Face []AttributeUserPwd `json:"face"`
	Palm []AttributeUserPwd `json:"palm"`
	Fp   []AttributeUserPwd `json:"fp"`
	Nfc  []AttributeUserPwd `json:"nfc"`
}

type AttributeUserPwd struct {
	Id   string `json:"id"`
	Data string `json:"data"`
	Enc  string `json:"enc"`
	Exp  int64  `json:"exp"`
}

var (
	ErrUserNotFound       = fmt.Errorf("user not found")
	ErrCredentialNotFound = fmt.Errorf("credential not found")
	ErrInvalidCredential  = fmt.Errorf("invalid type")
)

func UsersFromReported(value any) []AttributeUser {
	switch v := value.(type) {
	case []AttributeUser:
		return cloneUsers(v)
	case []any:
		users, err := DecodeUsers(v)
		if err == nil {
			return users
		}
	case []map[string]any:
		users, err := DecodeUsers(v)
		if err == nil {
			return users
		}
	case nil:
		return []AttributeUser{}
	}
	return []AttributeUser{}
}

func LastUserID(value any) string {
	users := UsersFromReported(value)
	if len(users) == 0 {
		return ""
	}
	return users[len(users)-1].Id
}

func AddUser(state *State, name string, role int) AttributeUser {
	snapshot := state.Snapshot()
	users := UsersFromReported(snapshot.Reported["Users"])
	user := AttributeUser{
		Id:   fmt.Sprintf("%d", time.Now().Unix()),
		Name: name,
		Role: role,
		Pwd:  []AttributeUserPwd{},
		Face: []AttributeUserPwd{},
		Palm: []AttributeUserPwd{},
		Fp:   []AttributeUserPwd{},
		Nfc:  []AttributeUserPwd{},
	}
	users = append(users, user)
	state.SetReported(map[string]any{"Users": users})
	return user
}

func UpdateUser(state *State, userID, name string, role int) error {
	snapshot := state.Snapshot()
	users := UsersFromReported(snapshot.Reported["Users"])
	for i := range users {
		if users[i].Id != userID {
			continue
		}
		users[i].Name = name
		users[i].Role = role
		state.SetReported(map[string]any{"Users": users})
		return nil
	}
	return ErrUserNotFound
}

func DelUser(state *State, userID string) error {
	snapshot := state.Snapshot()
	users := UsersFromReported(snapshot.Reported["Users"])
	for i := range users {
		if users[i].Id != userID {
			continue
		}
		users = append(users[:i], users[i+1:]...)
		state.SetReported(map[string]any{"Users": users})
		return nil
	}
	return ErrUserNotFound
}

func AddPwd(state *State, userID string, credentialType int, data, enc string, exp int64) (string, error) {
	if !isValidCredentialType(credentialType) {
		return "", ErrInvalidCredential
	}

	snapshot := state.Snapshot()
	users := UsersFromReported(snapshot.Reported["Users"])
	for i := range users {
		if users[i].Id != userID {
			continue
		}

		pwdID := fmt.Sprintf("%s_%d", userID, time.Now().Unix())
		entry := AttributeUserPwd{
			Id:   pwdID,
			Data: data,
			Enc:  enc,
			Exp:  exp,
		}

		switch credentialType {
		case 2:
			users[i].Pwd = append(users[i].Pwd, entry)
		case 3:
			users[i].Fp = append(users[i].Fp, entry)
		case 4:
			users[i].Face = append(users[i].Face, entry)
		case 6:
			users[i].Palm = append(users[i].Palm, entry)
		case 7:
			users[i].Nfc = append(users[i].Nfc, entry)
		case 1, 5:
		}

		state.SetReported(map[string]any{"Users": users})
		return pwdID, nil
	}
	return "", ErrUserNotFound
}

func UpdatePwd(state *State, userID string, credentialType int, pwdID, data, enc string, exp int64) error {
	if !isValidCredentialType(credentialType) {
		return ErrInvalidCredential
	}

	snapshot := state.Snapshot()
	users := UsersFromReported(snapshot.Reported["Users"])
	for i := range users {
		if users[i].Id != userID {
			continue
		}

		target, found := credentialListByType(&users[i], credentialType)
		if !found {
			return ErrCredentialNotFound
		}
		for idx := range *target {
			if (*target)[idx].Id != pwdID {
				continue
			}
			(*target)[idx].Data = data
			(*target)[idx].Enc = enc
			(*target)[idx].Exp = exp
			state.SetReported(map[string]any{"Users": users})
			return nil
		}
		return ErrCredentialNotFound
	}
	return ErrUserNotFound
}

func DelPwd(state *State, userID string, credentialType int, pwdID string) error {
	if !isValidCredentialType(credentialType) {
		return ErrInvalidCredential
	}

	snapshot := state.Snapshot()
	users := UsersFromReported(snapshot.Reported["Users"])
	for i := range users {
		if users[i].Id != userID {
			continue
		}

		target, found := credentialListByType(&users[i], credentialType)
		if !found {
			return ErrCredentialNotFound
		}
		for idx := range *target {
			if (*target)[idx].Id != pwdID {
				continue
			}
			*target = append((*target)[:idx], (*target)[idx+1:]...)
			state.SetReported(map[string]any{"Users": users})
			return nil
		}
		return ErrCredentialNotFound
	}
	return ErrUserNotFound
}

func cloneUsers(in []AttributeUser) []AttributeUser {
	out := make([]AttributeUser, len(in))
	for i := range in {
		out[i] = cloneUser(in[i])
	}
	return out
}

func cloneUser(in AttributeUser) AttributeUser {
	return AttributeUser{
		Id:   in.Id,
		Name: in.Name,
		Role: in.Role,
		Pwd:  clonePwdList(in.Pwd),
		Face: clonePwdList(in.Face),
		Palm: clonePwdList(in.Palm),
		Fp:   clonePwdList(in.Fp),
		Nfc:  clonePwdList(in.Nfc),
	}
}

func clonePwdList(in []AttributeUserPwd) []AttributeUserPwd {
	out := make([]AttributeUserPwd, len(in))
	copy(out, in)
	return out
}

func credentialListByType(user *AttributeUser, credentialType int) (*[]AttributeUserPwd, bool) {
	switch credentialType {
	case 2:
		return &user.Pwd, true
	case 3:
		return &user.Fp, true
	case 4:
		return &user.Face, true
	case 6:
		return &user.Palm, true
	case 7:
		return &user.Nfc, true
	default:
		return nil, false
	}
}

func isValidCredentialType(credentialType int) bool {
	return credentialType >= 1 && credentialType <= 7
}

func DecodeUsers(value any) ([]AttributeUser, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var users []AttributeUser
	if err := json.Unmarshal(raw, &users); err != nil {
		return nil, err
	}
	for i := range users {
		users[i] = cloneUser(users[i])
	}
	return users, nil
}
