package provisioning

import "testing"

func TestIsSuccessCode(t *testing.T) {
	t.Parallel()

	if !isSuccessCode(0) {
		t.Fatal("expected legacy success code 0 to be accepted")
	}
	if !isSuccessCode(1000) {
		t.Fatal("expected HAS success code 1000 to be accepted")
	}
	if isSuccessCode(2000) {
		t.Fatal("expected error code 2000 to be rejected")
	}
}
