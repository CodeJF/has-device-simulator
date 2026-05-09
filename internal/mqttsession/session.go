package mqttsession

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/jianfengxu/has-device-simulator/internal/config"
	"github.com/jianfengxu/has-device-simulator/internal/deviceprofile"
	"github.com/jianfengxu/has-device-simulator/internal/protocol"
	"github.com/jianfengxu/has-device-simulator/internal/provisioning"
)

type Session interface {
	Connect(ctx context.Context) error
	Disconnect(quiesce uint)
	SetHandlers(Handlers)
	PublishAttrPost(ctx context.Context, data map[string]any) error
	PublishAttrSetResp(ctx context.Context, resp protocol.ResultEnvelope) error
	PublishEvent(ctx context.Context, eventName string, data map[string]any) error
	PublishFuncResp(ctx context.Context, funcName string, resp protocol.FunctionResponse) error
}

type Handlers struct {
	OnAttrSet     func(context.Context, protocol.Envelope) error
	OnAttrGetResp func(context.Context, protocol.ResultEnvelope) error
	OnFunction    func(context.Context, protocol.FunctionRequest) error
}

type Client struct {
	logger   *slog.Logger
	profile  deviceprofile.Profile
	config   config.MQTTConfig
	creds    provisioning.MQTTCredentials
	handlers Handlers
	client   mqtt.Client
}

func New(logger *slog.Logger, cfg config.MQTTConfig, profile deviceprofile.Profile, creds provisioning.MQTTCredentials) *Client {
	c := &Client{
		logger:  logger,
		profile: profile,
		config:  cfg,
		creds:   creds,
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(creds.ClientID)
	opts.SetUsername(creds.Username)
	opts.SetPassword(creds.Password)
	logger.Info("mqtt login credentials", "username", creds.Username, "password", creds.Password)
	opts.SetKeepAlive(time.Duration(cfg.KeepaliveSeconds) * time.Second)
	opts.SetCleanSession(cfg.CleanSession)
	opts.SetAutoReconnect(true)
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		if err := c.subscribeAll(); err != nil {
			c.logger.Error("subscribe after connect failed", "err", err)
		}
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		c.logger.Warn("mqtt connection lost", "err", err)
	})

	c.client = mqtt.NewClient(opts)
	return c
}

func (c *Client) SetHandlers(handlers Handlers) {
	c.handlers = handlers
}

func (c *Client) Connect(_ context.Context) error {
	token := c.client.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("mqtt connect timeout")
	}
	return token.Error()
}

func (c *Client) Disconnect(quiesce uint) {
	if c.client != nil && c.client.IsConnected() {
		c.client.Disconnect(quiesce)
	}
}

func (c *Client) PublishAttrPost(_ context.Context, data map[string]any) error {
	payload := protocol.Envelope{
		MsgID: newMessageID(),
		Time:  time.Now().UnixMilli(),
	}
	raw, err := json.Marshal(struct {
		MsgID string         `json:"msg_id"`
		Time  int64          `json:"time"`
		Data  map[string]any `json:"data"`
	}{
		MsgID: payload.MsgID,
		Time:  payload.Time,
		Data:  data,
	})
	if err != nil {
		return err
	}
	return c.publish(protocol.ThingTopic(c.profile.Model, c.profile.UUID, "attr", "post"), raw)
}

func (c *Client) PublishAttrSetResp(_ context.Context, resp protocol.ResultEnvelope) error {
	raw, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return c.publish(protocol.ThingTopic(c.profile.Model, c.profile.UUID, "attr", "set", "resp"), raw)
}

func (c *Client) PublishEvent(_ context.Context, eventName string, data map[string]any) error {
	raw, err := json.Marshal(struct {
		MsgID string         `json:"msg_id"`
		Time  int64          `json:"time"`
		Data  map[string]any `json:"data"`
	}{
		MsgID: newMessageID(),
		Time:  time.Now().UnixMilli(),
		Data:  data,
	})
	if err != nil {
		return err
	}
	return c.publish(protocol.ThingTopic(c.profile.Model, c.profile.UUID, "event", eventName), raw)
}

func (c *Client) PublishFuncResp(_ context.Context, funcName string, resp protocol.FunctionResponse) error {
	raw, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return c.publish(protocol.ThingTopic(c.profile.Model, c.profile.UUID, "func", funcName, "resp"), raw)
}

func (c *Client) subscribeAll() error {
	subscriptions := map[string]mqtt.MessageHandler{
		protocol.ThingTopic(c.profile.Model, c.profile.UUID, "attr", "set"):         c.handleAttrSet,
		protocol.ThingTopic(c.profile.Model, c.profile.UUID, "attr", "get", "resp"): c.handleAttrGetResp,
		protocol.ThingTopic(c.profile.Model, c.profile.UUID, "func", "+"):           c.handleFunction,
	}

	for topic, handler := range subscriptions {
		token := c.client.Subscribe(topic, 1, handler)
		if !token.WaitTimeout(10 * time.Second) {
			return fmt.Errorf("mqtt subscribe timeout: %s", topic)
		}
		if err := token.Error(); err != nil {
			return fmt.Errorf("mqtt subscribe %s: %w", topic, err)
		}
		c.logger.Info("subscribed mqtt topic", "topic", topic)
	}

	return nil
}

func (c *Client) handleAttrSet(_ mqtt.Client, msg mqtt.Message) {
	if c.handlers.OnAttrSet == nil {
		return
	}
	env, err := protocol.DecodeEnvelope(msg.Payload())
	if err != nil {
		c.logger.Error("decode attr set failed", "err", err)
		return
	}
	if err := c.handlers.OnAttrSet(context.Background(), *env); err != nil {
		c.logger.Error("handle attr set failed", "err", err)
	}
}

func (c *Client) handleAttrGetResp(_ mqtt.Client, msg mqtt.Message) {
	if c.handlers.OnAttrGetResp == nil {
		return
	}
	var env protocol.ResultEnvelope
	if err := json.Unmarshal(msg.Payload(), &env); err != nil {
		c.logger.Error("decode attr get resp failed", "err", err)
		return
	}
	if err := c.handlers.OnAttrGetResp(context.Background(), env); err != nil {
		c.logger.Error("handle attr get resp failed", "err", err)
	}
}

func (c *Client) handleFunction(_ mqtt.Client, msg mqtt.Message) {
	if c.handlers.OnFunction == nil {
		return
	}
	funcName, ok := protocol.ParseFunctionTopic(msg.Topic())
	if !ok {
		c.logger.Warn("skip unknown func topic", "topic", msg.Topic())
		return
	}
	env, err := protocol.DecodeEnvelope(msg.Payload())
	if err != nil {
		c.logger.Error("decode func request failed", "err", err)
		return
	}
	req := protocol.FunctionRequest{
		Name:  funcName,
		MsgID: env.MsgID,
		Time:  env.Time,
		Data:  env.Data,
	}
	if err := c.handlers.OnFunction(context.Background(), req); err != nil {
		c.logger.Error("handle func request failed", "func", funcName, "err", err)
	}
}

func (c *Client) publish(topic string, payload []byte) error {
	token := c.client.Publish(topic, 1, false, payload)
	if !token.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("mqtt publish timeout: %s", topic)
	}
	return token.Error()
}

func newMessageID() string {
	return fmt.Sprintf("sim-%d", time.Now().UnixNano())
}
