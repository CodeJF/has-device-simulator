package protocol

import "testing"

func TestBuildDeviceSignatureUsesLegacyEncoding(t *testing.T) {
	t.Parallel()

	sign, err := BuildDeviceSignature("POST", "SL100", "device-req-id", "1715155200", "", "device-test-uuid", map[string]any{
		"uid":     "u_123",
		"mac":     "AA:BB:CC:DD:EE:FF",
		"zone":    "8.00",
		"version": "1.0.0",
	}, "device-model-secret")
	if err != nil {
		t.Fatalf("BuildDeviceSignature() error = %v", err)
	}

	const want = "ZWVhNzE3NThkZGQ5MDQ5Nzg5ZTMwYjBjMTUyNzA2YzhhN2U0ZjIyNmZmN2IxNjJlMTZjMzA2OWNlOWRjZTM4Nw=="
	if sign != want {
		t.Fatalf("BuildDeviceSignature() = %q, want %q", sign, want)
	}
}

func TestBuildDeviceSignatureIncludesEmptyBodyValuesLikeLegacyScript(t *testing.T) {
	t.Parallel()

	sign, err := BuildDeviceSignature("POST", "SL100", "req-1", "1715155200", "", "device-1", map[string]any{
		"mac":     "AA:BB",
		"remark":  "",
		"version": "1.0.0",
	}, "device-model-secret")
	if err != nil {
		t.Fatalf("BuildDeviceSignature() error = %v", err)
	}

	const want = "MTU1Y2I5NzliNjM5YTIwOGQyZGM1NzA3YjZiOTg5YTU1MWE4YjUyOWUxYzgyMGE3OTVkMDIxZDQ0ZGI0YTQ3ZA=="
	if sign != want {
		t.Fatalf("BuildDeviceSignature() with empty string = %q, want %q", sign, want)
	}
}

func TestBuildDeviceSignatureEncodesQueryValuesLikeLegacyScript(t *testing.T) {
	t.Parallel()

	sign, err := BuildDeviceSignature("GET", "SL100", "req-2", "1715155200", "", "device-2", map[string]any{
		"name": "front door",
		"tag":  "A&B",
	}, "device-model-secret")
	if err != nil {
		t.Fatalf("BuildDeviceSignature() error = %v", err)
	}

	const want = "NTI5YTlhMTYzNTY1NmUxOTdmZGUwMTU3YzcwMDk2YjJiOWFmZTk3NTg4NzdjODU5ZmJhZjQ3MjAzOTI0YTU0Zg=="
	if sign != want {
		t.Fatalf("BuildDeviceSignature() query encoding = %q, want %q", sign, want)
	}
}

func TestBuildDeviceSignatureIncludesUIDWhenPresent(t *testing.T) {
	t.Parallel()

	sign, err := BuildDeviceSignature("POST", "SL100", "abc123def4567890", "1715155200", "u_123", "device-2", map[string]any{
		"version": "1.0.0",
		"zone":    "8.00",
	}, "device-model-secret")
	if err != nil {
		t.Fatalf("BuildDeviceSignature() error = %v", err)
	}

	const want = "YTQzNzcyNTU0NDIyOGY3ZDYwOWQ5Y2U1YmZkMWQwYmExMjBiNzNlYjA5MDI2MzhlOTZjMDM3M2E4NDI5N2I3Mg=="
	if sign != want {
		t.Fatalf("BuildDeviceSignature() with uid = %q, want %q", sign, want)
	}
}
