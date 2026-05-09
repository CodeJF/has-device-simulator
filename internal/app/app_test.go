package app

import (
	"testing"

	"github.com/jianfengxu/has-device-simulator/internal/shadow"
)

func TestResolveScenarioDataReplacesLastUserID(t *testing.T) {
	state := shadow.New()
	user := shadow.AddUser(state, "Alice", 1)

	got := resolveScenarioData(map[string]any{
		"user_id": "$last_user_id",
		"type":    2,
		"data":    "123456",
	}, state.Snapshot())

	if got["user_id"] != user.Id {
		t.Fatalf("user_id = %v, want %v", got["user_id"], user.Id)
	}
	if got["type"] != 2 || got["data"] != "123456" {
		t.Fatalf("resolved data = %#v", got)
	}
}
