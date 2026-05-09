package config

import "testing"

func TestValidate(t *testing.T) {
	cfg := &AppConfig{
		Backend: BackendConfig{APIBaseURL: "http://localhost:8080"},
		MQTT:    MQTTConfig{Broker: "tcp://localhost:1883"},
		Device: DeviceConfig{
			Model:       "SL100",
			UUID:        "abc",
			AppID:       "smartlock",
			ModelSecret: "secret",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
