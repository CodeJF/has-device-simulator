package shadow

type UpgradeStatus struct {
	Step     int `json:"step"`
	Schedule int `json:"schedule"`
}

func NewUpgradeStatus(step int) UpgradeStatus {
	return UpgradeStatus{
		Step:     step,
		Schedule: step * 20,
	}
}
