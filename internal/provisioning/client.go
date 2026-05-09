package provisioning

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jianfengxu/has-device-simulator/internal/config"
	"github.com/jianfengxu/has-device-simulator/internal/deviceprofile"
	"github.com/jianfengxu/has-device-simulator/internal/protocol"
)

type MQTTCredentials struct {
	Username string
	Password string
	ClientID string
}

type Client interface {
	Bind(ctx context.Context, profile deviceprofile.Profile) (*MQTTCredentials, error)
	Login(ctx context.Context, profile deviceprofile.Profile) (*MQTTCredentials, error)
	ResolveMQTTCredentials(ctx context.Context, profile deviceprofile.Profile) (*MQTTCredentials, error)
}

type HTTPClient struct {
	baseURL     string
	timeout     time.Duration
	credentials config.CredentialsConfig
	httpClient  *http.Client
}

type apiResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

const (
	legacySuccessCode = 0
	hasSuccessCode    = 1000
)

func NewHTTPClient(cfg *config.AppConfig) *HTTPClient {
	timeout := time.Duration(cfg.Backend.TimeoutSeconds) * time.Second
	return &HTTPClient{
		baseURL:     strings.TrimRight(cfg.Backend.APIBaseURL, "/"),
		timeout:     timeout,
		credentials: cfg.Credentials,
		httpClient:  &http.Client{Timeout: timeout},
	}
}

func (c *HTTPClient) Bind(ctx context.Context, profile deviceprofile.Profile) (*MQTTCredentials, error) {
	payload := map[string]any{
		"uid":     profile.UID,
		"mac":     profile.MAC,
		"zone":    profile.Zone,
		"version": profile.Version,
	}

	resp, err := c.doDeviceRequest(ctx, http.MethodPost, "/v1/device/bind", profile, payload, false)
	if err != nil {
		return nil, err
	}

	creds, err := c.extractCredentials(resp.Data, profile)
	if err == nil {
		return creds, nil
	}

	return c.ResolveMQTTCredentials(ctx, profile)
}

func (c *HTTPClient) Login(ctx context.Context, profile deviceprofile.Profile) (*MQTTCredentials, error) {
	payload := map[string]any{
		"zone":    profile.Zone,
		"version": profile.Version,
	}

	resp, err := c.doDeviceRequest(ctx, http.MethodPost, "/v1/device/login", profile, payload, true)
	if err != nil {
		return nil, err
	}

	creds, err := c.extractCredentials(resp.Data, profile)
	if err == nil {
		return creds, nil
	}

	return c.ResolveMQTTCredentials(ctx, profile)
}

func (c *HTTPClient) ResolveMQTTCredentials(_ context.Context, profile deviceprofile.Profile) (*MQTTCredentials, error) {
	switch c.credentials.Mode {
	case "", "fixed":
		return &MQTTCredentials{
			Username: profile.UUID,
			Password: c.credentials.FixedPassword,
			ClientID: profile.ClientID,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported credentials mode %q", c.credentials.Mode)
	}
}

func (c *HTTPClient) doDeviceRequest(ctx context.Context, method, path string, profile deviceprofile.Profile, body map[string]any, includeUIDHeader bool) (*apiResponse, error) {
	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}

	requestID := uuid.NewString()
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	signUID := ""
	if includeUIDHeader {
		signUID = profile.UID
	}
	sign, err := protocol.BuildDeviceSignature(method, profile.Model, requestID, timestamp, signUID, profile.UUID, body, profile.Secret)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("model", profile.Model)
	req.Header.Set("uuid", profile.UUID)
	req.Header.Set("appid", profile.AppID)
	req.Header.Set("timestamp", timestamp)
	req.Header.Set("request_id", requestID)
	req.Header.Set("sign", sign)
	if includeUIDHeader {
		req.Header.Set("uid", profile.UID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var out apiResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if !isSuccessCode(out.Code) {
		return nil, fmt.Errorf("api error code=%d msg=%s", out.Code, out.Msg)
	}

	return &out, nil
}

func isSuccessCode(code int) bool {
	return code == legacySuccessCode || code == hasSuccessCode
}

func (c *HTTPClient) extractCredentials(raw json.RawMessage, profile deviceprofile.Profile) (*MQTTCredentials, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, fmt.Errorf("empty bind data")
	}

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("decode bind data: %w", err)
	}

	username := firstNonEmptyString(data, "mqtt_username", "username", "uuid")
	password := firstNonEmptyString(data, "mqtt_password", "password")
	if username == "" || password == "" {
		return nil, fmt.Errorf("bind response does not contain mqtt credentials")
	}

	return &MQTTCredentials{
		Username: username,
		Password: password,
		ClientID: profile.ClientID,
	}, nil
}

func firstNonEmptyString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			if s := strings.TrimSpace(fmt.Sprint(value)); s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}
