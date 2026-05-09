package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Envelope struct {
	MsgID string          `json:"msg_id"`
	Time  int64           `json:"time"`
	Data  json.RawMessage `json:"data,omitempty"`
}

type ResultEnvelope struct {
	MsgID  string `json:"msg_id"`
	Time   int64  `json:"time"`
	Result int    `json:"result"`
	Msg    string `json:"msg,omitempty"`
	Data   any    `json:"data,omitempty"`
}

type FunctionRequest struct {
	Name  string
	MsgID string
	Time  int64
	Data  json.RawMessage
}

type FunctionResponse struct {
	MsgID   string `json:"msg_id"`
	Time    int64  `json:"time"`
	Result  int    `json:"result"`
	ErrCode int    `json:"err_code,omitempty"`
	Msg     string `json:"msg,omitempty"`
	Status  string `json:"status,omitempty"`
	Data    any    `json:"data,omitempty"`
}

type EventPayload map[string]any

func ThingTopic(model, uuid string, parts ...string) string {
	base := []string{"", "thing", model, uuid}
	base = append(base, parts...)
	return strings.Join(base, "/")
}

func ParseFunctionTopic(topic string) (string, bool) {
	parts := strings.Split(strings.Trim(topic, "/"), "/")
	if len(parts) != 5 {
		return "", false
	}
	if parts[0] != "thing" || parts[3] != "func" {
		return "", false
	}
	return parts[4], true
}

func DecodeEnvelope(payload []byte) (*Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	return &env, nil
}
