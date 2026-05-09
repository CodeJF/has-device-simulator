package protocol

import "testing"

func TestThingTopic(t *testing.T) {
	got := ThingTopic("SL100", "uuid-1", "func", "Lock", "resp")
	want := "/thing/SL100/uuid-1/func/Lock/resp"
	if got != want {
		t.Fatalf("ThingTopic() = %q, want %q", got, want)
	}
}

func TestParseFunctionTopic(t *testing.T) {
	got, ok := ParseFunctionTopic("/thing/SL100/uuid-1/func/Lock")
	if !ok || got != "Lock" {
		t.Fatalf("ParseFunctionTopic() = (%q, %v)", got, ok)
	}
}
