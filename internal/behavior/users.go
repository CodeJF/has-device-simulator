package behavior

import (
	"context"

	"github.com/jianfengxu/has-device-simulator/internal/protocol"
	"github.com/jianfengxu/has-device-simulator/internal/shadow"
)

type createUserPayload struct {
	Name string `json:"name"`
	Role int    `json:"role"`
}

type updateUserPayload struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Role   int    `json:"role"`
}

type passwordPayload struct {
	UserID string `json:"user_id"`
	Type   int    `json:"type"`
	Data   string `json:"data"`
	Enc    string `json:"enc"`
	Exp    int64  `json:"exp"`
	PwdID  string `json:"pwd_id"`
}

func (e *DefaultEngine) handleLockLike(ctx context.Context, req protocol.FunctionRequest, lockState string, lockStatus int) error {
	if err := e.publishAttrPatch(ctx, map[string]any{
		"LockState":  lockState,
		"LockStatus": lockStatus,
	}); err != nil {
		return err
	}
	return e.publishFuncResp(ctx, req.Name, successResponse(req.MsgID, "success", nil))
}

func (e *DefaultEngine) handleCreateUser(ctx context.Context, req protocol.FunctionRequest) error {
	var payload createUserPayload
	if err := decodeStruct(req.Data, &payload); err != nil {
		return e.publishFuncResp(ctx, req.Name, failureResponse(req.MsgID, "invalid", errInvalidData, "invalid payload"))
	}

	user := shadow.AddUser(e.shadow, payload.Name, payload.Role)
	if err := e.publishUsers(ctx); err != nil {
		return err
	}
	return e.publishFuncResp(ctx, req.Name, successResponse(req.MsgID, "success", map[string]any{"id": user.Id}))
}

func (e *DefaultEngine) handleUpdateUser(ctx context.Context, req protocol.FunctionRequest) error {
	var payload updateUserPayload
	if err := decodeStruct(req.Data, &payload); err != nil {
		return e.publishFuncResp(ctx, req.Name, failureResponse(req.MsgID, "invalid", errInvalidData, "invalid payload"))
	}
	if payload.UserID == "" {
		return e.publishFuncResp(ctx, req.Name, failureResponse(req.MsgID, "invalid", errInvalidData, "user_id is required"))
	}

	if err := shadow.UpdateUser(e.shadow, payload.UserID, payload.Name, payload.Role); err != nil {
		return e.publishFuncResp(ctx, req.Name, mapShadowError(req.MsgID, err))
	}
	if err := e.publishUsers(ctx); err != nil {
		return err
	}
	return e.publishFuncResp(ctx, req.Name, successResponse(req.MsgID, "success", map[string]any{"id": payload.UserID}))
}

func (e *DefaultEngine) handleDelUser(ctx context.Context, req protocol.FunctionRequest) error {
	var payload updateUserPayload
	if err := decodeStruct(req.Data, &payload); err != nil {
		return e.publishFuncResp(ctx, req.Name, failureResponse(req.MsgID, "invalid", errInvalidData, "invalid payload"))
	}
	if payload.UserID == "" {
		return e.publishFuncResp(ctx, req.Name, failureResponse(req.MsgID, "invalid", errInvalidData, "user_id is required"))
	}

	if err := shadow.DelUser(e.shadow, payload.UserID); err != nil {
		return e.publishFuncResp(ctx, req.Name, mapShadowError(req.MsgID, err))
	}
	if err := e.publishUsers(ctx); err != nil {
		return err
	}
	if err := e.publisher.PublishEvent(ctx, "DelUser", map[string]any{"user_id": payload.UserID}); err != nil {
		return err
	}
	if err := e.publisher.PublishEvent(ctx, "SetUsers", map[string]any{"users": shadow.UsersFromReported(e.shadow.Snapshot().Reported["Users"])}); err != nil {
		return err
	}
	return e.publishFuncResp(ctx, req.Name, successResponse(req.MsgID, "success", map[string]any{"id": payload.UserID}))
}

func (e *DefaultEngine) handleAddPwd(ctx context.Context, req protocol.FunctionRequest) error {
	var payload passwordPayload
	if err := decodeStruct(req.Data, &payload); err != nil {
		return e.publishFuncResp(ctx, req.Name, failureResponse(req.MsgID, "invalid", errInvalidData, "invalid payload"))
	}
	if payload.UserID == "" {
		return e.publishFuncResp(ctx, req.Name, failureResponse(req.MsgID, "invalid", errInvalidData, "user_id is required"))
	}

	pwdID, err := shadow.AddPwd(e.shadow, payload.UserID, payload.Type, payload.Data, payload.Enc, payload.Exp)
	if err != nil {
		return e.publishFuncResp(ctx, req.Name, mapShadowError(req.MsgID, err))
	}
	if err := e.publishUsers(ctx); err != nil {
		return err
	}
	event := map[string]any{"user_id": payload.UserID, "type": payload.Type, "pwd_id": pwdID}
	if err := e.publisher.PublishEvent(ctx, "AddPwd", event); err != nil {
		return err
	}
	if err := e.publisher.PublishEvent(ctx, "SetUsers", map[string]any{"users": shadow.UsersFromReported(e.shadow.Snapshot().Reported["Users"])}); err != nil {
		return err
	}
	return e.publishFuncResp(ctx, req.Name, successResponse(req.MsgID, "success", map[string]any{"id": pwdID}))
}

func (e *DefaultEngine) handleUpdatePwd(ctx context.Context, req protocol.FunctionRequest) error {
	var payload passwordPayload
	if err := decodeStruct(req.Data, &payload); err != nil {
		return e.publishFuncResp(ctx, req.Name, failureResponse(req.MsgID, "invalid", errInvalidData, "invalid payload"))
	}
	if payload.UserID == "" || payload.PwdID == "" {
		return e.publishFuncResp(ctx, req.Name, failureResponse(req.MsgID, "invalid", errInvalidData, "user_id and pwd_id are required"))
	}

	if err := shadow.UpdatePwd(e.shadow, payload.UserID, payload.Type, payload.PwdID, payload.Data, payload.Enc, payload.Exp); err != nil {
		return e.publishFuncResp(ctx, req.Name, mapShadowError(req.MsgID, err))
	}
	if err := e.publishUsers(ctx); err != nil {
		return err
	}
	return e.publishFuncResp(ctx, req.Name, successResponse(req.MsgID, "success", nil))
}

func (e *DefaultEngine) handleDelPwd(ctx context.Context, req protocol.FunctionRequest) error {
	var payload passwordPayload
	if err := decodeStruct(req.Data, &payload); err != nil {
		return e.publishFuncResp(ctx, req.Name, failureResponse(req.MsgID, "invalid", errInvalidData, "invalid payload"))
	}
	if payload.UserID == "" || payload.PwdID == "" {
		return e.publishFuncResp(ctx, req.Name, failureResponse(req.MsgID, "invalid", errInvalidData, "user_id and pwd_id are required"))
	}

	if err := shadow.DelPwd(e.shadow, payload.UserID, payload.Type, payload.PwdID); err != nil {
		return e.publishFuncResp(ctx, req.Name, mapShadowError(req.MsgID, err))
	}
	if err := e.publishUsers(ctx); err != nil {
		return err
	}
	event := map[string]any{"user_id": payload.UserID, "type": payload.Type, "pwd_id": payload.PwdID}
	if err := e.publisher.PublishEvent(ctx, "DelPwd", event); err != nil {
		return err
	}
	if err := e.publisher.PublishEvent(ctx, "SetUsers", map[string]any{"users": shadow.UsersFromReported(e.shadow.Snapshot().Reported["Users"])}); err != nil {
		return err
	}
	return e.publishFuncResp(ctx, req.Name, successResponse(req.MsgID, "success", nil))
}

func mapShadowError(msgID string, err error) protocol.FunctionResponse {
	switch err {
	case shadow.ErrInvalidCredential:
		return failureResponse(msgID, "invalid", errInvalidData, "invalid type")
	case shadow.ErrUserNotFound:
		return failureResponse(msgID, "not_found", errNotFound, "user not found")
	case shadow.ErrCredentialNotFound:
		return failureResponse(msgID, "not_found", errNotFound, "credential not found")
	default:
		return failureResponse(msgID, "error", 50000, err.Error())
	}
}
