package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type AppConfig struct {
	Backend     BackendConfig     `mapstructure:"backend"`
	MQTT        MQTTConfig        `mapstructure:"mqtt"`
	Device      DeviceConfig      `mapstructure:"device"`
	Behavior    BehaviorConfig    `mapstructure:"behavior"`
	Credentials CredentialsConfig `mapstructure:"credentials"`
}

type BackendConfig struct {
	APIBaseURL     string `mapstructure:"api_base_url"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

type MQTTConfig struct {
	Broker           string `mapstructure:"broker"`
	KeepaliveSeconds int    `mapstructure:"keepalive_seconds"`
	CleanSession     bool   `mapstructure:"clean_session"`
	ClientID         string `mapstructure:"client_id"`
}

type DeviceConfig struct {
	Model       string `mapstructure:"model"`
	UUID        string `mapstructure:"uuid"`
	UID         string `mapstructure:"uid"`
	AppID       string `mapstructure:"appid"`
	MAC         string `mapstructure:"mac"`
	Zone        string `mapstructure:"zone"`
	Version     string `mapstructure:"version"`
	ModelSecret string `mapstructure:"model_secret"`
}

type BehaviorConfig struct {
	HeartbeatSeconds int `mapstructure:"heartbeat_seconds"`
	AttrPostSeconds  int `mapstructure:"attr_post_seconds"`
}

type CredentialsConfig struct {
	Mode          string `mapstructure:"mode"`
	FixedPassword string `mapstructure:"fixed_password"`
}

func Load(path string) (*AppConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvPrefix("SIM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg AppConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("backend.timeout_seconds", 10)
	v.SetDefault("mqtt.keepalive_seconds", 30)
	v.SetDefault("mqtt.clean_session", true)
	v.SetDefault("behavior.heartbeat_seconds", 60)
	v.SetDefault("behavior.attr_post_seconds", 0)
	v.SetDefault("credentials.mode", "fixed")
	v.SetDefault("credentials.fixed_password", "debug-fixed-password")
}

func (c *AppConfig) Validate() error {
	switch {
	case c.Backend.APIBaseURL == "":
		return fmt.Errorf("backend.api_base_url is required")
	case c.MQTT.Broker == "":
		return fmt.Errorf("mqtt.broker is required")
	case c.Device.Model == "":
		return fmt.Errorf("device.model is required")
	case c.Device.UUID == "":
		return fmt.Errorf("device.uuid is required")
	case c.Device.AppID == "":
		return fmt.Errorf("device.appid is required")
	case c.Device.ModelSecret == "":
		return fmt.Errorf("device.model_secret is required")
	}
	return nil
}
