package behavior

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jianfengxu/has-device-simulator/internal/protocol"
	"github.com/jianfengxu/has-device-simulator/internal/shadow"
)

const (
	errUnsupported = 40001
	errInvalidData = 40002
	errNotFound    = 40401
	errConflict    = 40901
)

type Publisher interface {
	Disconnect(quiesce uint)
	PublishAttrPost(ctx context.Context, data map[string]any) error
	PublishAttrSetResp(ctx context.Context, resp protocol.ResultEnvelope) error
	PublishFuncResp(ctx context.Context, funcName string, resp protocol.FunctionResponse) error
	PublishEvent(ctx context.Context, eventName string, data map[string]any) error
}

type Engine interface {
	HandleAttrSet(ctx context.Context, env protocol.Envelope) error
	HandleAttrGetResp(ctx context.Context, env protocol.ResultEnvelope) error
	HandleFunc(ctx context.Context, req protocol.FunctionRequest) error
	TriggerEvent(ctx context.Context, eventName string) error
	TriggerEventWithData(ctx context.Context, eventName string, override map[string]any) error
}

type EventFactory interface {
	Build(eventName string, state shadow.Snapshot) (protocol.EventPayload, error)
}

type Options struct {
	OTAStepInterval time.Duration
}

type DefaultEngine struct {
	logger          *slog.Logger
	shadow          *shadow.State
	publisher       Publisher
	events          EventFactory
	otaStepInterval time.Duration

	otaMu      sync.Mutex
	otaRunning bool
}

func New(logger *slog.Logger, state *shadow.State, publisher Publisher) *DefaultEngine {
	return NewWithOptions(logger, state, publisher, Options{})
}

func NewWithOptions(logger *slog.Logger, state *shadow.State, publisher Publisher, opts Options) *DefaultEngine {
	if opts.OTAStepInterval <= 0 {
		opts.OTAStepInterval = 500 * time.Millisecond
	}

	return &DefaultEngine{
		logger:          logger,
		shadow:          state,
		publisher:       publisher,
		events:          StaticEventFactory{},
		otaStepInterval: opts.OTAStepInterval,
	}
}

func (e *DefaultEngine) HandleAttrSet(ctx context.Context, env protocol.Envelope) error {
	values, err := decodeMap(env.Data)
	if err != nil {
		return fmt.Errorf("decode attr set data: %w", err)
	}
	values = normalizeAttrPatch(values)
	if len(values) > 0 {
		e.shadow.SetDesired(values)
		e.shadow.SetReported(values)
	}

	if err := e.publisher.PublishAttrSetResp(ctx, protocol.ResultEnvelope{
		MsgID:  env.MsgID,
		Time:   time.Now().UnixMilli(),
		Result: 1,
		Msg:    "ok",
	}); err != nil {
		return err
	}
	if len(values) > 0 {
		return e.publisher.PublishAttrPost(ctx, values)
	}
	return nil
}

func (e *DefaultEngine) HandleAttrGetResp(_ context.Context, env protocol.ResultEnvelope) error {
	if env.Result != 1 {
		return nil
	}

	values, ok := mapFromAny(env.Data)
	if !ok {
		return nil
	}
	values = normalizeAttrPatch(values)
	if len(values) > 0 {
		e.shadow.SetReported(values)
	}
	return nil
}

func (e *DefaultEngine) HandleFunc(ctx context.Context, req protocol.FunctionRequest) error {
	e.logger.Info("handle function request", "func", req.Name, "msg_id", req.MsgID)

	switch req.Name {
	case "Lock":
		return e.handleLockLike(ctx, req, "locked", 1)
	case "Unlock":
		return e.handleLockLike(ctx, req, "unlocked", 0)
	case "CreateUser":
		return e.handleCreateUser(ctx, req)
	case "UpdateUser":
		return e.handleUpdateUser(ctx, req)
	case "DelUser":
		return e.handleDelUser(ctx, req)
	case "AddPwd":
		return e.handleAddPwd(ctx, req)
	case "UpdatePwd":
		return e.handleUpdatePwd(ctx, req)
	case "DelPwd":
		return e.handleDelPwd(ctx, req)
	case "OtaUpgrade":
		return e.handleOtaUpgrade(ctx, req)
	case "UnBind":
		return e.handleUnbind(ctx, req)
	case "Reboot":
		return e.handleReboot(ctx, req)
	default:
		resp := failureResponse(req.MsgID, "unsupported", errUnsupported, "function not implemented in simulator")
		resp.Data = map[string]any{"reason": "function not implemented in simulator"}
		return e.publishFuncResp(ctx, req.Name, resp)
	}
}

func (e *DefaultEngine) TriggerEvent(ctx context.Context, eventName string) error {
	return e.TriggerEventWithData(ctx, eventName, nil)
}

func (e *DefaultEngine) TriggerEventWithData(ctx context.Context, eventName string, override map[string]any) error {
	state := e.shadow.Snapshot()
	payload, err := e.events.Build(eventName, state)
	if err != nil {
		return err
	}
	if len(override) > 0 {
		for key, value := range override {
			payload[key] = value
		}
	}

	if patch := attrPatchForEvent(eventName, payload); len(patch) > 0 {
		e.shadow.SetReported(patch)
		if err := e.publisher.PublishAttrPost(ctx, patch); err != nil {
			return err
		}
	}

	return e.publisher.PublishEvent(ctx, eventName, payload)
}

type StaticEventFactory struct{}

func (StaticEventFactory) Build(eventName string, state shadow.Snapshot) (protocol.EventPayload, error) {
	switch eventName {
	case "Ring":
		return protocol.EventPayload{
			"source":   "doorbell",
			"face_id":  "sim-face-001",
			"ringing":  true,
			"battery":  firstInt(state.Reported["Battery"], 80),
			"scenario": "manual",
		}, nil
	case "RingStop":
		return protocol.EventPayload{
			"reason":   "timeout",
			"ringing":  false,
			"scenario": "manual",
		}, nil
	case "OpenDoor":
		return protocol.EventPayload{
			"method":   "fingerprint",
			"result":   "success",
			"lock":     firstString(state.Reported["LockState"], "locked"),
			"scenario": "manual",
		}, nil
	case "LowBattery":
		return protocol.EventPayload{
			"battery":   20,
			"threshold": 20,
			"result":    "warning",
			"scenario":  "manual",
		}, nil
	case "UpgradeStart":
		return protocol.EventPayload{
			"step":     1,
			"schedule": 20,
			"version":  firstString(state.Reported["Version"], ""),
		}, nil
	case "UpgradeResult":
		return protocol.EventPayload{
			"result":  1,
			"msg":     "OTA升级成功",
			"version": firstString(state.Reported["Version"], ""),
			"time":    time.Now().UnixMilli(),
		}, nil
	case "Reboot":
		return protocol.EventPayload{
			"reason":  "manual",
			"version": firstString(state.Reported["Version"], ""),
			"time":    time.Now().UnixMilli(),
		}, nil
	case "AddPwd":
		userID, pwdType, pwdID := manualCredentialEventData(state.Reported["Users"])
		return protocol.EventPayload{
			"user_id": userID,
			"type":    pwdType,
			"pwd_id":  pwdID,
		}, nil
	case "DelPwd":
		userID, pwdType, pwdID := manualCredentialEventData(state.Reported["Users"])
		return protocol.EventPayload{
			"user_id": userID,
			"type":    pwdType,
			"pwd_id":  pwdID,
		}, nil
	case "DelUser":
		userID := shadow.LastUserID(state.Reported["Users"])
		if userID == "" {
			userID = "manual-user"
		}
		return protocol.EventPayload{
			"user_id": userID,
		}, nil
	case "SetUsers":
		return protocol.EventPayload{
			"users": shadow.UsersFromReported(state.Reported["Users"]),
		}, nil
	case "UnBind":
		return protocol.EventPayload{
			"reason": "manual",
		}, nil
	default:
		return nil, fmt.Errorf("unsupported event %q", eventName)
	}
}

func attrPatchForEvent(eventName string, payload map[string]any) map[string]any {
	switch eventName {
	case "Ring":
		return map[string]any{"Ringing": true}
	case "RingStop":
		return map[string]any{"Ringing": false}
	case "OpenDoor":
		return map[string]any{"DoorLastAction": "open_success"}
	case "LowBattery":
		return map[string]any{"Battery": firstInt(payload["battery"], 20)}
	case "UpgradeStart":
		return map[string]any{
			"UpgradeStatus": shadow.UpgradeStatus{
				Step:     firstInt(payload["step"], 1),
				Schedule: firstInt(payload["schedule"], 20),
			},
		}
	default:
		return nil
	}
}

func (e *DefaultEngine) publishFuncResp(ctx context.Context, funcName string, resp protocol.FunctionResponse) error {
	return e.publisher.PublishFuncResp(ctx, funcName, resp)
}

func (e *DefaultEngine) publishAttrPatch(ctx context.Context, patch map[string]any) error {
	if len(patch) == 0 {
		return nil
	}
	patch = normalizeAttrPatch(patch)
	e.shadow.SetReported(patch)
	return e.publisher.PublishAttrPost(ctx, patch)
}

func (e *DefaultEngine) publishUsers(ctx context.Context) error {
	return e.publishAttrPatch(ctx, map[string]any{
		"Users": shadow.UsersFromReported(e.shadow.Snapshot().Reported["Users"]),
	})
}

func decodeMap(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}, nil
	}

	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func decodeStruct(raw json.RawMessage, target any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return json.Unmarshal(raw, target)
}

func mapFromAny(value any) (map[string]any, bool) {
	switch v := value.(type) {
	case map[string]any:
		return v, true
	default:
		return nil, false
	}
}

func normalizeAttrPatch(values map[string]any) map[string]any {
	if len(values) == 0 {
		return values
	}

	normalized := make(map[string]any, len(values)+1)
	for key, value := range values {
		normalized[key] = value
	}

	if lockState, ok := normalized["LockState"].(string); ok {
		switch lockState {
		case "locked":
			normalized["LockStatus"] = 1
		case "unlocked":
			normalized["LockStatus"] = 0
		}
	}

	if lockStatus, ok := normalized["LockStatus"]; ok {
		if firstInt(lockStatus, 1) == 0 {
			normalized["LockState"] = "unlocked"
		} else {
			normalized["LockState"] = "locked"
		}
	}

	if users, ok := normalized["Users"]; ok {
		if decoded, err := shadow.DecodeUsers(users); err == nil {
			normalized["Users"] = decoded
		}
	}

	if status, ok := normalized["UpgradeStatus"].(map[string]any); ok {
		normalized["UpgradeStatus"] = shadow.UpgradeStatus{
			Step:     firstInt(status["step"], 0),
			Schedule: firstInt(status["schedule"], 0),
		}
	}

	return normalized
}

func successResponse(msgID, status string, data any) protocol.FunctionResponse {
	return protocol.FunctionResponse{
		MsgID:  msgID,
		Time:   time.Now().UnixMilli(),
		Result: 1,
		Status: status,
		Msg:    "ok",
		Data:   data,
	}
}

func failureResponse(msgID, status string, errCode int, message string) protocol.FunctionResponse {
	return protocol.FunctionResponse{
		MsgID:   msgID,
		Time:    time.Now().UnixMilli(),
		Result:  0,
		Status:  status,
		ErrCode: errCode,
		Msg:     message,
	}
}

func firstInt(value any, fallback int) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return fallback
	}
}

func firstString(value any, fallback string) string {
	if s, ok := value.(string); ok && s != "" {
		return s
	}
	return fallback
}

func manualCredentialEventData(value any) (userID string, credentialType int, pwdID string) {
	userID = "manual-user"
	credentialType = 2
	pwdID = "manual-pwd"

	users := shadow.UsersFromReported(value)
	if len(users) == 0 {
		return userID, credentialType, pwdID
	}

	last := users[len(users)-1]
	if last.Id != "" {
		userID = last.Id
	}

	switch {
	case len(last.Pwd) > 0:
		return userID, 2, last.Pwd[0].Id
	case len(last.Fp) > 0:
		return userID, 3, last.Fp[0].Id
	case len(last.Face) > 0:
		return userID, 4, last.Face[0].Id
	case len(last.Palm) > 0:
		return userID, 6, last.Palm[0].Id
	case len(last.Nfc) > 0:
		return userID, 7, last.Nfc[0].Id
	default:
		return userID, credentialType, pwdID
	}
}
