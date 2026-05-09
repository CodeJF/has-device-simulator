package behavior

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/jianfengxu/has-device-simulator/internal/protocol"
	"github.com/jianfengxu/has-device-simulator/internal/shadow"
)

type publishedEvent struct {
	name string
	data map[string]any
}

type functionCall struct {
	name string
	resp protocol.FunctionResponse
}

type mockPublisher struct {
	attrPosts     []map[string]any
	events        []publishedEvent
	attrSetResp   *protocol.ResultEnvelope
	functionCalls []functionCall
	disconnects   int
}

func (m *mockPublisher) Disconnect(_ uint) {
	m.disconnects++
}

func (m *mockPublisher) PublishAttrPost(_ context.Context, data map[string]any) error {
	m.attrPosts = append(m.attrPosts, cloneMap(data))
	return nil
}

func (m *mockPublisher) PublishAttrSetResp(_ context.Context, resp protocol.ResultEnvelope) error {
	m.attrSetResp = &resp
	return nil
}

func (m *mockPublisher) PublishFuncResp(_ context.Context, funcName string, resp protocol.FunctionResponse) error {
	m.functionCalls = append(m.functionCalls, functionCall{name: funcName, resp: resp})
	return nil
}

func (m *mockPublisher) PublishEvent(_ context.Context, eventName string, data map[string]any) error {
	m.events = append(m.events, publishedEvent{name: eventName, data: cloneMap(data)})
	return nil
}

func TestHandleAttrSetPublishesAttrPost(t *testing.T) {
	state := shadow.New()
	pub := &mockPublisher{}
	engine := New(slog.Default(), state, pub)

	env := protocol.Envelope{
		MsgID: "msg-1",
		Data:  []byte(`{"Online":1,"LockState":"unlocked"}`),
	}
	if err := engine.HandleAttrSet(context.Background(), env); err != nil {
		t.Fatalf("HandleAttrSet() error = %v", err)
	}
	if pub.attrSetResp == nil || pub.attrSetResp.Result != 1 {
		t.Fatalf("attr set resp = %+v", pub.attrSetResp)
	}
	if len(pub.attrPosts) != 1 {
		t.Fatalf("attr posts len = %d, want 1", len(pub.attrPosts))
	}
	if got := firstInt(pub.attrPosts[0]["LockStatus"], -1); got != 0 {
		t.Fatalf("LockStatus = %d, want 0", got)
	}
}

func TestHandleAttrGetRespOverwritesUsersAndUpgradeStatus(t *testing.T) {
	state := shadow.New()
	pub := &mockPublisher{}
	engine := New(slog.Default(), state, pub)

	err := engine.HandleAttrGetResp(context.Background(), protocol.ResultEnvelope{
		MsgID:  "get-1",
		Result: 1,
		Data: map[string]any{
			"Users": []any{
				map[string]any{
					"id":   "u1",
					"name": "Alice",
					"role": 1,
					"pwd": []any{
						map[string]any{"id": "p1", "data": "123456", "enc": "", "exp": 0},
					},
					"face": []any{},
					"palm": []any{},
					"fp":   []any{},
					"nfc":  []any{},
				},
			},
			"UpgradeStatus": map[string]any{"step": 2, "schedule": 40},
		},
	})
	if err != nil {
		t.Fatalf("HandleAttrGetResp() error = %v", err)
	}

	snapshot := state.Snapshot()
	users := shadow.UsersFromReported(snapshot.Reported["Users"])
	if len(users) != 1 || users[0].Id != "u1" || len(users[0].Pwd) != 1 || users[0].Pwd[0].Id != "p1" {
		t.Fatalf("users after attr/get/resp = %#v", users)
	}
	status, ok := snapshot.Reported["UpgradeStatus"].(shadow.UpgradeStatus)
	if !ok {
		t.Fatalf("UpgradeStatus type = %T, want shadow.UpgradeStatus", snapshot.Reported["UpgradeStatus"])
	}
	if status.Step != 2 || status.Schedule != 40 {
		t.Fatalf("UpgradeStatus = %+v", status)
	}

	if err := engine.HandleFunc(context.Background(), protocol.FunctionRequest{
		Name:  "AddPwd",
		MsgID: "get-2",
		Data:  rawJSON(map[string]any{"user_id": "u1", "type": 7, "data": "nfc-001", "enc": "", "exp": 0}),
	}); err != nil {
		t.Fatalf("AddPwd after attr/get/resp error = %v", err)
	}

	users = shadow.UsersFromReported(state.Snapshot().Reported["Users"])
	if len(users) != 1 || len(users[0].Nfc) != 1 || users[0].Nfc[0].Data != "nfc-001" {
		t.Fatalf("users after follow-up AddPwd = %#v", users)
	}
}

func TestTriggerEventRingAndStop(t *testing.T) {
	state := shadow.New()
	pub := &mockPublisher{}
	engine := New(slog.Default(), state, pub)

	if err := engine.TriggerEvent(context.Background(), "Ring"); err != nil {
		t.Fatalf("TriggerEvent(Ring) error = %v", err)
	}
	if err := engine.TriggerEvent(context.Background(), "RingStop"); err != nil {
		t.Fatalf("TriggerEvent(RingStop) error = %v", err)
	}

	if len(pub.events) != 2 || pub.events[0].name != "Ring" || pub.events[1].name != "RingStop" {
		t.Fatalf("published events = %#v", pub.events)
	}
	if got := state.Snapshot().Reported["Ringing"]; got != false {
		t.Fatalf("Ringing after RingStop = %v, want false", got)
	}
	if len(pub.attrPosts) != 2 {
		t.Fatalf("attr posts len = %d, want 2", len(pub.attrPosts))
	}
}

func TestTriggerEventWithDataOverride(t *testing.T) {
	state := shadow.NewWithVersion("1.2.3")
	pub := &mockPublisher{}
	engine := New(slog.Default(), state, pub)

	if err := engine.TriggerEventWithData(context.Background(), "UpgradeStart", map[string]any{
		"step":     2,
		"schedule": 40,
		"version":  "2.0.0",
	}); err != nil {
		t.Fatalf("TriggerEventWithData() error = %v", err)
	}

	if len(pub.events) != 1 {
		t.Fatalf("events len = %d, want 1", len(pub.events))
	}
	if firstInt(pub.events[0].data["step"], -1) != 2 || firstInt(pub.events[0].data["schedule"], -1) != 40 || firstString(pub.events[0].data["version"], "") != "2.0.0" {
		t.Fatalf("event payload = %#v", pub.events[0].data)
	}
	status, ok := state.Snapshot().Reported["UpgradeStatus"].(shadow.UpgradeStatus)
	if !ok {
		t.Fatalf("UpgradeStatus type = %T", state.Snapshot().Reported["UpgradeStatus"])
	}
	if status.Step != 2 || status.Schedule != 40 {
		t.Fatalf("UpgradeStatus = %+v", status)
	}
}

func TestHandleFuncCreateUserAddAndDeletePwd(t *testing.T) {
	state := shadow.New()
	pub := &mockPublisher{}
	engine := New(slog.Default(), state, pub)

	if err := engine.HandleFunc(context.Background(), protocol.FunctionRequest{
		Name:  "CreateUser",
		MsgID: "m1",
		Data:  rawJSON(map[string]any{"name": "Alice", "role": 1}),
	}); err != nil {
		t.Fatalf("CreateUser error = %v", err)
	}

	users := shadow.UsersFromReported(state.Snapshot().Reported["Users"])
	if len(users) != 1 {
		t.Fatalf("users len after create = %d, want 1", len(users))
	}
	userID := users[0].Id
	if userID == "" {
		t.Fatal("expected generated user id")
	}

	if err := engine.HandleFunc(context.Background(), protocol.FunctionRequest{
		Name:  "AddPwd",
		MsgID: "m2",
		Data:  rawJSON(map[string]any{"user_id": userID, "type": 2, "data": "123456", "enc": "", "exp": 0}),
	}); err != nil {
		t.Fatalf("AddPwd error = %v", err)
	}

	users = shadow.UsersFromReported(state.Snapshot().Reported["Users"])
	if len(users[0].Pwd) != 1 {
		t.Fatalf("pwd len after add = %d, want 1", len(users[0].Pwd))
	}
	pwdID := users[0].Pwd[0].Id

	if err := engine.HandleFunc(context.Background(), protocol.FunctionRequest{
		Name:  "DelPwd",
		MsgID: "m3",
		Data:  rawJSON(map[string]any{"user_id": userID, "type": 2, "pwd_id": pwdID}),
	}); err != nil {
		t.Fatalf("DelPwd error = %v", err)
	}

	users = shadow.UsersFromReported(state.Snapshot().Reported["Users"])
	if len(users[0].Pwd) != 0 {
		t.Fatalf("pwd len after delete = %d, want 0", len(users[0].Pwd))
	}

	if len(pub.functionCalls) != 3 {
		t.Fatalf("func responses len = %d, want 3", len(pub.functionCalls))
	}
	if pub.functionCalls[0].resp.Result != 1 || pub.functionCalls[1].resp.Result != 1 || pub.functionCalls[2].resp.Result != 1 {
		t.Fatalf("func responses = %#v", pub.functionCalls)
	}
	if len(pub.attrPosts) < 3 {
		t.Fatalf("attr posts len = %d, want at least 3", len(pub.attrPosts))
	}
	if !hasEvent(pub.events, "AddPwd") || !hasEvent(pub.events, "DelPwd") || !hasEvent(pub.events, "SetUsers") {
		t.Fatalf("events = %#v", pub.events)
	}
}

func TestHandleFuncAddPwdTypeOneAndFiveSucceedWithoutSlot(t *testing.T) {
	state := shadow.New()
	pub := &mockPublisher{}
	engine := New(slog.Default(), state, pub)
	user := shadow.AddUser(state, "Alice", 1)

	for _, credentialType := range []int{1, 5} {
		if err := engine.HandleFunc(context.Background(), protocol.FunctionRequest{
			Name:  "AddPwd",
			MsgID: "special-type",
			Data:  rawJSON(map[string]any{"user_id": user.Id, "type": credentialType, "data": "noop", "enc": "", "exp": 0}),
		}); err != nil {
			t.Fatalf("AddPwd type=%d error = %v", credentialType, err)
		}
	}

	users := shadow.UsersFromReported(state.Snapshot().Reported["Users"])
	if len(users) != 1 {
		t.Fatalf("users len = %d, want 1", len(users))
	}
	if len(users[0].Pwd)+len(users[0].Fp)+len(users[0].Face)+len(users[0].Palm)+len(users[0].Nfc) != 0 {
		t.Fatalf("expected no credential slots for type 1/5, got %#v", users[0])
	}
	if len(pub.functionCalls) != 2 || pub.functionCalls[0].resp.Result != 1 || pub.functionCalls[1].resp.Result != 1 {
		t.Fatalf("function calls = %#v", pub.functionCalls)
	}
}

func TestHandleFuncLockUnlockMirrorsState(t *testing.T) {
	state := shadow.New()
	pub := &mockPublisher{}
	engine := New(slog.Default(), state, pub)

	if err := engine.HandleFunc(context.Background(), protocol.FunctionRequest{Name: "Unlock", MsgID: "u1"}); err != nil {
		t.Fatalf("Unlock error = %v", err)
	}
	snapshot := state.Snapshot()
	if snapshot.Reported["LockState"] != "unlocked" || firstInt(snapshot.Reported["LockStatus"], -1) != 0 {
		t.Fatalf("unlock state = %+v", snapshot.Reported)
	}

	if err := engine.HandleFunc(context.Background(), protocol.FunctionRequest{Name: "Lock", MsgID: "l1"}); err != nil {
		t.Fatalf("Lock error = %v", err)
	}
	snapshot = state.Snapshot()
	if snapshot.Reported["LockState"] != "locked" || firstInt(snapshot.Reported["LockStatus"], -1) != 1 {
		t.Fatalf("lock state = %+v", snapshot.Reported)
	}
}

func TestHandleFuncOtaUpgradePublishesProgress(t *testing.T) {
	state := shadow.NewWithVersion("1.0.0")
	pub := &mockPublisher{}
	engine := NewWithOptions(slog.Default(), state, pub, Options{OTAStepInterval: 10 * time.Millisecond})

	if err := engine.HandleFunc(context.Background(), protocol.FunctionRequest{
		Name:  "OtaUpgrade",
		MsgID: "ota-1",
		Data:  rawJSON(map[string]any{"version": "1.1.0", "md5": "x", "expire_time": 1715999999, "uri": "http://x"}),
	}); err != nil {
		t.Fatalf("OtaUpgrade error = %v", err)
	}

	waitFor(t, 300*time.Millisecond, func() bool {
		return hasEvent(pub.events, "UpgradeResult")
	})

	var progressPosts int
	for _, post := range pub.attrPosts {
		if _, ok := post["UpgradeStatus"]; ok {
			progressPosts++
		}
	}
	if progressPosts < 6 {
		t.Fatalf("upgrade status attr posts = %d, want at least 6", progressPosts)
	}
	if countEvents(pub.events, "UpgradeStart") != 5 {
		t.Fatalf("UpgradeStart events = %d, want 5", countEvents(pub.events, "UpgradeStart"))
	}
	if countEvents(pub.events, "UpgradeResult") != 1 {
		t.Fatalf("UpgradeResult events = %d, want 1", countEvents(pub.events, "UpgradeResult"))
	}
	if got := firstString(state.Snapshot().Reported["Version"], ""); got != "1.1.0" {
		t.Fatalf("version after ota = %q, want 1.1.0", got)
	}
}

func TestHandleFuncOtaUpgradeConflictReturns40901(t *testing.T) {
	state := shadow.NewWithVersion("1.0.0")
	pub := &mockPublisher{}
	engine := NewWithOptions(slog.Default(), state, pub, Options{OTAStepInterval: 100 * time.Millisecond})
	req := protocol.FunctionRequest{
		Name:  "OtaUpgrade",
		MsgID: "ota-conflict-1",
		Data:  rawJSON(map[string]any{"version": "1.1.0", "md5": "x", "expire_time": 1715999999, "uri": "http://x"}),
	}

	if err := engine.HandleFunc(context.Background(), req); err != nil {
		t.Fatalf("first OtaUpgrade error = %v", err)
	}
	if err := engine.HandleFunc(context.Background(), protocol.FunctionRequest{
		Name:  "OtaUpgrade",
		MsgID: "ota-conflict-2",
		Data:  req.Data,
	}); err != nil {
		t.Fatalf("second OtaUpgrade error = %v", err)
	}

	if len(pub.functionCalls) < 2 {
		t.Fatalf("function calls len = %d, want at least 2", len(pub.functionCalls))
	}
	resp := pub.functionCalls[1].resp
	if resp.Result != 0 || resp.ErrCode != errConflict || resp.Msg != "upgrade in progress" {
		t.Fatalf("ota conflict resp = %+v", resp)
	}
}

func TestHandleFuncUnbindResetsState(t *testing.T) {
	state := shadow.NewWithVersion("1.0.0")
	state.SetReported(map[string]any{"Online": 1, "Battery": 10})
	pub := &mockPublisher{}
	engine := NewWithOptions(slog.Default(), state, pub, Options{OTAStepInterval: 10 * time.Millisecond})

	if err := engine.HandleFunc(context.Background(), protocol.FunctionRequest{Name: "UnBind", MsgID: "ub-1"}); err != nil {
		t.Fatalf("UnBind error = %v", err)
	}
	if len(pub.functionCalls) != 1 || pub.functionCalls[0].name != "UnBind" {
		t.Fatalf("function calls = %#v", pub.functionCalls)
	}
	if len(pub.events) == 0 || pub.events[0].name != "UnBind" {
		t.Fatalf("events = %#v", pub.events)
	}

	waitFor(t, 1500*time.Millisecond, func() bool {
		return pub.disconnects == 1
	})

	snapshot := state.Snapshot()
	if firstInt(snapshot.Reported["Online"], -1) != 0 {
		t.Fatalf("Online after unbind = %v, want 0", snapshot.Reported["Online"])
	}
	if firstInt(snapshot.Reported["Battery"], -1) != 80 {
		t.Fatalf("Battery after unbind = %v, want 80", snapshot.Reported["Battery"])
	}
}

func TestHandleFuncRebootPublishesEventAndAttrPost(t *testing.T) {
	state := shadow.NewWithVersion("1.0.0")
	pub := &mockPublisher{}
	engine := New(slog.Default(), state, pub)

	if err := engine.HandleFunc(context.Background(), protocol.FunctionRequest{Name: "Reboot", MsgID: "rb-1"}); err != nil {
		t.Fatalf("Reboot error = %v", err)
	}

	waitFor(t, time.Second, func() bool {
		return hasEvent(pub.events, "Reboot") && len(pub.attrPosts) >= 2
	})
}

func TestHandleFuncUnsupportedReturnsResultZero(t *testing.T) {
	state := shadow.New()
	pub := &mockPublisher{}
	engine := New(slog.Default(), state, pub)

	if err := engine.HandleFunc(context.Background(), protocol.FunctionRequest{Name: "UnknownFunc", MsgID: "msg-3"}); err != nil {
		t.Fatalf("HandleFunc(UnknownFunc) error = %v", err)
	}
	if len(pub.functionCalls) != 1 {
		t.Fatalf("function calls len = %d, want 1", len(pub.functionCalls))
	}
	if pub.functionCalls[0].resp.Result != 0 || pub.functionCalls[0].resp.Status != "unsupported" {
		t.Fatalf("func unsupported resp = %+v", pub.functionCalls[0].resp)
	}
	data, ok := pub.functionCalls[0].resp.Data.(map[string]any)
	if !ok || firstString(data["reason"], "") != "function not implemented in simulator" {
		t.Fatalf("func unsupported data = %#v", pub.functionCalls[0].resp.Data)
	}
}

func TestHandleFuncAddPwdInvalidTypeReturns40002(t *testing.T) {
	state := shadow.New()
	pub := &mockPublisher{}
	engine := New(slog.Default(), state, pub)
	user := shadow.AddUser(state, "Alice", 1)

	if err := engine.HandleFunc(context.Background(), protocol.FunctionRequest{
		Name:  "AddPwd",
		MsgID: "m-invalid",
		Data:  rawJSON(map[string]any{"user_id": user.Id, "type": 9, "data": "123456", "enc": "", "exp": 0}),
	}); err != nil {
		t.Fatalf("AddPwd invalid type error = %v", err)
	}

	if len(pub.functionCalls) != 1 {
		t.Fatalf("function calls len = %d, want 1", len(pub.functionCalls))
	}
	resp := pub.functionCalls[0].resp
	if resp.Result != 0 || resp.ErrCode != errInvalidData || resp.Msg != "invalid type" {
		t.Fatalf("invalid type resp = %+v", resp)
	}
}

func TestHandleFuncUpdateUserMissingReturns40401(t *testing.T) {
	state := shadow.New()
	pub := &mockPublisher{}
	engine := New(slog.Default(), state, pub)

	if err := engine.HandleFunc(context.Background(), protocol.FunctionRequest{
		Name:  "UpdateUser",
		MsgID: "m-missing",
		Data:  rawJSON(map[string]any{"user_id": "missing", "name": "Bob", "role": 1}),
	}); err != nil {
		t.Fatalf("UpdateUser missing error = %v", err)
	}

	if len(pub.functionCalls) != 1 {
		t.Fatalf("function calls len = %d, want 1", len(pub.functionCalls))
	}
	resp := pub.functionCalls[0].resp
	if resp.Result != 0 || resp.ErrCode != errNotFound || resp.Msg != "user not found" {
		t.Fatalf("missing user resp = %+v", resp)
	}
}

func TestTriggerEventManualDerivedPayloads(t *testing.T) {
	state := shadow.NewWithVersion("1.2.3")
	user := shadow.AddUser(state, "Alice", 1)
	pwdID, err := shadow.AddPwd(state, user.Id, 2, "123456", "", 0)
	if err != nil {
		t.Fatalf("seed AddPwd error = %v", err)
	}
	pub := &mockPublisher{}
	engine := New(slog.Default(), state, pub)

	for _, eventName := range []string{"UpgradeStart", "UpgradeResult", "AddPwd", "DelPwd", "DelUser", "UnBind"} {
		if err := engine.TriggerEvent(context.Background(), eventName); err != nil {
			t.Fatalf("TriggerEvent(%s) error = %v", eventName, err)
		}
	}

	assertEventData(t, pub.events, "UpgradeStart", "step", 1)
	assertEventData(t, pub.events, "UpgradeStart", "schedule", 20)
	assertEventData(t, pub.events, "UpgradeStart", "version", "1.2.3")
	assertEventData(t, pub.events, "UpgradeResult", "version", "1.2.3")
	assertEventData(t, pub.events, "AddPwd", "user_id", user.Id)
	assertEventData(t, pub.events, "AddPwd", "pwd_id", pwdID)
	assertEventData(t, pub.events, "DelPwd", "user_id", user.Id)
	assertEventData(t, pub.events, "DelPwd", "pwd_id", pwdID)
	assertEventData(t, pub.events, "DelUser", "user_id", user.Id)
	assertEventData(t, pub.events, "UnBind", "reason", "manual")
}

func rawJSON(value map[string]any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func hasEvent(events []publishedEvent, name string) bool {
	return countEvents(events, name) > 0
}

func countEvents(events []publishedEvent, name string) int {
	count := 0
	for _, event := range events {
		if event.name == name {
			count++
		}
	}
	return count
}

func assertEventData(t *testing.T, events []publishedEvent, name, key string, want any) {
	t.Helper()

	for _, event := range events {
		if event.name != name {
			continue
		}
		got := event.data[key]
		switch wantTyped := want.(type) {
		case int:
			if firstInt(got, -1) != wantTyped {
				t.Fatalf("%s.%s = %v, want %v", name, key, got, want)
			}
		case string:
			if firstString(got, "") != wantTyped {
				t.Fatalf("%s.%s = %v, want %v", name, key, got, want)
			}
		default:
			if got != want {
				t.Fatalf("%s.%s = %v, want %v", name, key, got, want)
			}
		}
		return
	}

	t.Fatalf("event %s not found", name)
}

func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not satisfied before timeout")
}
