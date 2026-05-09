package deviceprofile

import (
	"fmt"

	"github.com/jianfengxu/has-device-simulator/internal/config"
)

type Profile struct {
	Model    string
	UUID     string
	UID      string
	AppID    string
	MAC      string
	Zone     string
	Version  string
	Secret   string
	ClientID string
}

func FromConfig(cfg config.DeviceConfig, explicitClientID string) Profile {
	clientID := explicitClientID
	if clientID == "" {
		clientID = fmt.Sprintf("%s_%s", cfg.Model, cfg.UUID)
	}

	return Profile{
		Model:    cfg.Model,
		UUID:     cfg.UUID,
		UID:      cfg.UID,
		AppID:    cfg.AppID,
		MAC:      cfg.MAC,
		Zone:     cfg.Zone,
		Version:  cfg.Version,
		Secret:   cfg.ModelSecret,
		ClientID: clientID,
	}
}
