package behavior

import (
	"context"
	"time"

	"github.com/jianfengxu/has-device-simulator/internal/protocol"
)

func (e *DefaultEngine) handleUnbind(ctx context.Context, req protocol.FunctionRequest) error {
	if err := e.publishFuncResp(ctx, req.Name, successResponse(req.MsgID, "unbinding", map[string]any{"accepted": true})); err != nil {
		return err
	}
	if err := e.publisher.PublishEvent(ctx, "UnBind", map[string]any{"reason": "manual"}); err != nil {
		return err
	}

	go func() {
		time.Sleep(time.Second)
		e.publisher.Disconnect(250)
		version := firstString(e.shadow.Snapshot().Reported["Version"], "")
		e.shadow.Reset(version)
		e.logger.Info("device unbound; waiting for external bind")
	}()

	return nil
}

func (e *DefaultEngine) handleReboot(ctx context.Context, req protocol.FunctionRequest) error {
	if err := e.publishFuncResp(ctx, req.Name, successResponse(req.MsgID, "rebooting", nil)); err != nil {
		return err
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		bg := context.Background()
		version := firstString(e.shadow.Snapshot().Reported["Version"], "")
		if err := e.publisher.PublishEvent(bg, "Reboot", map[string]any{
			"reason":  "manual",
			"version": version,
			"time":    time.Now().UnixMilli(),
		}); err != nil {
			e.logger.Error("publish reboot event failed", "err", err)
			return
		}
		if err := e.publishAttrPatch(bg, map[string]any{"Online": 1}); err != nil {
			e.logger.Error("publish reboot online failed", "err", err)
			return
		}
		if err := e.publishAttrPatch(bg, map[string]any{"Version": version}); err != nil {
			e.logger.Error("publish reboot version failed", "err", err)
		}
	}()

	return nil
}
