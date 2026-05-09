package behavior

import (
	"context"
	"time"

	"github.com/jianfengxu/has-device-simulator/internal/protocol"
	"github.com/jianfengxu/has-device-simulator/internal/shadow"
)

type otaUpgradePayload struct {
	Version    string `json:"version"`
	MD5        string `json:"md5"`
	ExpireTime int64  `json:"expire_time"`
	URI        string `json:"uri"`
}

func (e *DefaultEngine) handleOtaUpgrade(ctx context.Context, req protocol.FunctionRequest) error {
	var payload otaUpgradePayload
	if err := decodeStruct(req.Data, &payload); err != nil {
		return e.publishFuncResp(ctx, req.Name, failureResponse(req.MsgID, "invalid", errInvalidData, "invalid payload"))
	}

	e.otaMu.Lock()
	if e.otaRunning {
		e.otaMu.Unlock()
		return e.publishFuncResp(ctx, req.Name, failureResponse(req.MsgID, "conflict", errConflict, "upgrade in progress"))
	}
	e.otaRunning = true
	e.otaMu.Unlock()

	if err := e.publishFuncResp(ctx, req.Name, successResponse(req.MsgID, "accepted", map[string]any{"accepted": true})); err != nil {
		e.finishOTA()
		return err
	}

	go e.runOTA(payload)
	return nil
}

func (e *DefaultEngine) runOTA(payload otaUpgradePayload) {
	defer e.finishOTA()

	ctx := context.Background()
	for step := 1; step <= 5; step++ {
		time.Sleep(e.otaStepInterval)

		status := shadow.NewUpgradeStatus(step)
		if err := e.publishAttrPatch(ctx, map[string]any{"UpgradeStatus": status}); err != nil {
			e.logger.Error("publish ota status failed", "step", step, "err", err)
			return
		}
		if err := e.publisher.PublishEvent(ctx, "UpgradeStart", map[string]any{
			"step":     status.Step,
			"schedule": status.Schedule,
			"version":  payload.Version,
		}); err != nil {
			e.logger.Error("publish ota event failed", "step", step, "err", err)
			return
		}
	}

	if err := e.publisher.PublishEvent(ctx, "UpgradeResult", map[string]any{
		"result":  1,
		"msg":     "OTA升级成功",
		"version": payload.Version,
		"time":    time.Now().UnixMilli(),
	}); err != nil {
		e.logger.Error("publish ota result failed", "err", err)
		return
	}

	if err := e.publishAttrPatch(ctx, map[string]any{
		"Version":       payload.Version,
		"UpgradeStatus": shadow.UpgradeStatus{},
	}); err != nil {
		e.logger.Error("publish ota final state failed", "err", err)
	}
}

func (e *DefaultEngine) finishOTA() {
	e.otaMu.Lock()
	defer e.otaMu.Unlock()
	e.otaRunning = false
}
