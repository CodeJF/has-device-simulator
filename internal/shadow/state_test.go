package shadow

import "testing"

func TestStateSnapshot(t *testing.T) {
	s := New()
	s.SetDesired(map[string]any{"Online": 1})
	s.SetReported(map[string]any{"Battery": 90})

	snapshot := s.Snapshot()
	if snapshot.Desired["Online"] != 1 {
		t.Fatalf("desired Online = %v", snapshot.Desired["Online"])
	}
	if snapshot.Reported["Battery"] != 90 {
		t.Fatalf("reported Battery = %v", snapshot.Reported["Battery"])
	}
}
