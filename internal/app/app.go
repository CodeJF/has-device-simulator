package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jianfengxu/has-device-simulator/internal/behavior"
	"github.com/jianfengxu/has-device-simulator/internal/config"
	"github.com/jianfengxu/has-device-simulator/internal/deviceprofile"
	"github.com/jianfengxu/has-device-simulator/internal/mqttsession"
	"github.com/jianfengxu/has-device-simulator/internal/protocol"
	"github.com/jianfengxu/has-device-simulator/internal/provisioning"
	"github.com/jianfengxu/has-device-simulator/internal/scenario"
	"github.com/jianfengxu/has-device-simulator/internal/shadow"
)

type SimulatorApp struct {
	Config       *config.AppConfig
	Profile      deviceprofile.Profile
	Logger       *slog.Logger
	Provisioning provisioning.Client
	Shadow       *shadow.State
	session      mqttsession.Session
	behavior     behavior.Engine
}

func New(cfg *config.AppConfig) (*SimulatorApp, error) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
	profile := deviceprofile.FromConfig(cfg.Device, cfg.MQTT.ClientID)
	provisionClient := provisioning.NewHTTPClient(cfg)
	shadowState := shadow.New()
	shadowState = shadow.NewWithVersion(profile.Version)

	return &SimulatorApp{
		Config:       cfg,
		Profile:      profile,
		Logger:       logger,
		Provisioning: provisionClient,
		Shadow:       shadowState,
	}, nil
}

func (a *SimulatorApp) Bind(ctx context.Context) error {
	if _, err := a.Provisioning.Bind(ctx, a.Profile); err != nil {
		return err
	}
	loginCreds, err := a.Provisioning.Login(ctx, a.Profile)
	if err != nil {
		return err
	}

	a.Logger.Info("bind/login completed", "mqtt_username", loginCreds.Username, "client_id", loginCreds.ClientID)
	return nil
}

func (a *SimulatorApp) Run(ctx context.Context) error {
	if err := a.setupSession(ctx); err != nil {
		return err
	}
	defer a.session.Disconnect(250)

	if err := a.publishOnline(ctx); err != nil {
		return err
	}

	a.Logger.Info("simulator online", "uuid", a.Profile.UUID, "model", a.Profile.Model)
	<-ctx.Done()
	return nil
}

func (a *SimulatorApp) TriggerEvent(ctx context.Context, name string) error {
	if err := a.setupSession(ctx); err != nil {
		return err
	}
	defer a.session.Disconnect(250)

	if err := a.publishOnline(ctx); err != nil {
		return err
	}
	if err := a.behavior.TriggerEvent(ctx, name); err != nil {
		return err
	}
	a.Logger.Info("event published", "name", name)
	return nil
}

func (a *SimulatorApp) RunScenario(ctx context.Context, name string) error {
	def, ok := lookupScenario(name)
	if !ok {
		return fmt.Errorf("unknown scenario %q; supported: %+v", name, scenario.DefaultScenarios)
	}

	if err := a.setupSession(ctx); err != nil {
		return err
	}
	defer a.session.Disconnect(250)

	for _, step := range def.Steps {
		switch step.Action {
		case "bind", "connect", "login":
		case "online":
			if err := a.publishOnline(ctx); err != nil {
				return err
			}
			case "event":
				if err := a.behavior.TriggerEventWithData(ctx, step.Target, resolveScenarioData(step.Data, a.Shadow.Snapshot())); err != nil {
					return err
				}
		case "func":
			req, err := buildScenarioFunctionRequest(step, a.Shadow.Snapshot())
			if err != nil {
				return err
			}
			if err := a.behavior.HandleFunc(ctx, req); err != nil {
				return err
			}
		case "sleep":
			select {
			case <-time.After(step.Wait):
			case <-ctx.Done():
				return ctx.Err()
			}
		default:
			return fmt.Errorf("unsupported scenario step %q", step.Action)
		}
	}

	a.Logger.Info("scenario completed", "name", name)
	return nil
}

func (a *SimulatorApp) setupSession(ctx context.Context) error {
	if _, err := a.Provisioning.Bind(ctx, a.Profile); err != nil {
		return err
	}

	loginCreds, err := a.Provisioning.Login(ctx, a.Profile)
	if err != nil {
		return err
	}

	session := mqttsession.New(a.Logger, a.Config.MQTT, a.Profile, *loginCreds)
	engine := behavior.New(a.Logger, a.Shadow, session)
	session.SetHandlers(mqttsession.Handlers{
		OnAttrSet: engine.HandleAttrSet,
		OnAttrGetResp: func(ctx context.Context, env protocol.ResultEnvelope) error {
			return engine.HandleAttrGetResp(ctx, env)
		},
		OnFunction: func(ctx context.Context, req protocol.FunctionRequest) error {
			return engine.HandleFunc(ctx, req)
		},
	})

	if err := session.Connect(ctx); err != nil {
		return err
	}

	a.session = session
	a.behavior = engine
	return nil
}

func (a *SimulatorApp) publishOnline(ctx context.Context) error {
	patch := map[string]any{
		"Online":  1,
		"Version": firstReportedString(a.Shadow.Snapshot().Reported["Version"], a.Profile.Version),
	}
	a.Shadow.SetReported(patch)
	return a.session.PublishAttrPost(ctx, patch)
}

func lookupScenario(name string) (scenario.Definition, bool) {
	for _, item := range scenario.DefaultScenarios {
		if item.Name == name {
			return item, true
		}
	}
	return scenario.Definition{}, false
}

func buildScenarioFunctionRequest(step scenario.Step, snapshot shadow.Snapshot) (protocol.FunctionRequest, error) {
	data := resolveScenarioData(step.Data, snapshot)
	raw, err := json.Marshal(data)
	if err != nil {
		return protocol.FunctionRequest{}, fmt.Errorf("marshal scenario func data: %w", err)
	}

	return protocol.FunctionRequest{
		Name:  step.Target,
		MsgID: fmt.Sprintf("sim-%d", time.Now().UnixNano()),
		Time:  time.Now().UnixMilli(),
		Data:  raw,
	}, nil
}

func resolveScenarioData(input map[string]any, snapshot shadow.Snapshot) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}

	out := make(map[string]any, len(input))
	lastUserID := shadow.LastUserID(snapshot.Reported["Users"])
	for key, value := range input {
		if s, ok := value.(string); ok && s == "$last_user_id" {
			out[key] = lastUserID
			continue
		}
		out[key] = value
	}
	return out
}

func firstReportedString(value any, fallback string) string {
	if s, ok := value.(string); ok && s != "" {
		return s
	}
	return fallback
}
