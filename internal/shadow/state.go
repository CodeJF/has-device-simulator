package shadow

import "sync"

type State struct {
	mu       sync.RWMutex
	Desired  map[string]any
	Reported map[string]any
	Metadata map[string]any
}

type Snapshot struct {
	Desired  map[string]any
	Reported map[string]any
	Metadata map[string]any
}

func New() *State {
	return &State{
		Desired:  map[string]any{},
		Reported: DefaultReportedState(""),
		Metadata: map[string]any{},
	}
}

func NewWithVersion(version string) *State {
	return &State{
		Desired:  map[string]any{},
		Reported: DefaultReportedState(version),
		Metadata: map[string]any{},
	}
}

func (s *State) SetDesired(values map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, value := range values {
		s.Desired[key] = cloneValue(value)
	}
}

func (s *State) SetReported(values map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, value := range values {
		s.Reported[key] = cloneValue(value)
	}
}

func (s *State) Reset(version string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Desired = map[string]any{}
	s.Reported = DefaultReportedState(version)
	s.Metadata = map[string]any{}
}

func (s *State) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	desired := make(map[string]any, len(s.Desired))
	reported := make(map[string]any, len(s.Reported))
	metadata := make(map[string]any, len(s.Metadata))
	for key, value := range s.Desired {
		desired[key] = cloneValue(value)
	}
	for key, value := range s.Reported {
		reported[key] = cloneValue(value)
	}
	for key, value := range s.Metadata {
		metadata[key] = cloneValue(value)
	}
	return Snapshot{
		Desired:  desired,
		Reported: reported,
		Metadata: metadata,
	}
}

func DefaultReportedState(version string) map[string]any {
	return map[string]any{
		"Online":         0,
		"Version":        version,
		"LockState":      "locked",
		"LockStatus":     1,
		"Battery":        80,
		"LowBattery":     20,
		"Charging":       0,
		"Ringing":        false,
		"DoorLastAction": "",
		"Volume":         65,
		"SleepState":     0,
		"LightStatus":    1,
		"UpgradeStatus":  UpgradeStatus{},
		"WIFI": map[string]any{
			"ssid": "",
			"pwd":  "",
		},
		"RSSI":  -100,
		"Users": []AttributeUser{},
	}
}

func cloneValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			out[key] = cloneValue(child)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = cloneValue(child)
		}
		return out
	case []AttributeUser:
		return cloneUsers(v)
	case AttributeUser:
		return cloneUser(v)
	case []AttributeUserPwd:
		out := make([]AttributeUserPwd, len(v))
		copy(out, v)
		return out
	default:
		return v
	}
}
