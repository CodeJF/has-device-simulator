package scenario

import (
	"context"
	"time"
)

type Definition struct {
	Name        string
	Description string
	Steps       []Step
}

type Runner interface {
	Run(ctx context.Context, name string) error
}

type Step struct {
	Action string
	Target string
	Data   map[string]any
	Wait   time.Duration
}

var DefaultScenarios = []Definition{
	{
		Name:        "bind-online",
		Description: "bind device, login, connect mqtt, and publish Online=1 once",
		Steps: []Step{
			{Action: "bind"},
			{Action: "connect"},
			{Action: "login"},
			{Action: "online"},
		},
	},
	{
		Name:        "bind-online-ring",
		Description: "bind device, go online, then publish Ring event",
		Steps: []Step{
			{Action: "bind"},
			{Action: "connect"},
			{Action: "login"},
			{Action: "online"},
			{Action: "event", Target: "Ring"},
		},
	},
	{
		Name:        "bind-online-open-door",
		Description: "bind device, go online, then publish OpenDoor event",
		Steps: []Step{
			{Action: "bind"},
			{Action: "connect"},
			{Action: "login"},
			{Action: "online"},
			{Action: "event", Target: "OpenDoor"},
		},
	},
	{
		Name:        "bind-online-add-pwd",
		Description: "bind, go online, create one user, then add password",
		Steps: []Step{
			{Action: "bind"},
			{Action: "connect"},
			{Action: "login"},
			{Action: "online"},
			{Action: "func", Target: "CreateUser", Data: map[string]any{"name": "Alice", "role": 1}},
			{Action: "sleep", Wait: 200 * time.Millisecond},
			{Action: "func", Target: "AddPwd", Data: map[string]any{"user_id": "$last_user_id", "type": 2, "data": "123456", "enc": "", "exp": 0}},
		},
	},
	{
		Name:        "bind-online-ota",
		Description: "bind, go online, then run simulated ota",
		Steps: []Step{
			{Action: "bind"},
			{Action: "connect"},
			{Action: "login"},
			{Action: "online"},
			{Action: "func", Target: "OtaUpgrade", Data: map[string]any{"version": "1.1.0", "md5": "x", "expire_time": 1715999999, "uri": "http://x"}},
			{Action: "sleep", Wait: 4 * time.Second},
		},
	},
	{
		Name:        "bind-online-unbind",
		Description: "bind, go online, then unbind without exiting process",
		Steps: []Step{
			{Action: "bind"},
			{Action: "connect"},
			{Action: "login"},
			{Action: "online"},
			{Action: "func", Target: "UnBind"},
		},
	},
}
