package shadow

import "testing"

func TestUserLifecycle(t *testing.T) {
	state := New()

	user := AddUser(state, "Alice", 1)
	if user.Id == "" {
		t.Fatal("expected generated user id")
	}

	if err := UpdateUser(state, user.Id, "Alice2", 2); err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}

	users := UsersFromReported(state.Snapshot().Reported["Users"])
	if len(users) != 1 {
		t.Fatalf("users len = %d, want 1", len(users))
	}
	if users[0].Name != "Alice2" || users[0].Role != 2 {
		t.Fatalf("updated user = %+v", users[0])
	}

	if err := DelUser(state, user.Id); err != nil {
		t.Fatalf("DelUser() error = %v", err)
	}
	if users := UsersFromReported(state.Snapshot().Reported["Users"]); len(users) != 0 {
		t.Fatalf("users len after delete = %d, want 0", len(users))
	}
}

func TestAddPwdMapsToCorrectBuckets(t *testing.T) {
	state := New()
	user := AddUser(state, "Alice", 1)

	tests := []struct {
		credentialType int
		check          func(AttributeUser) int
	}{
		{credentialType: 2, check: func(user AttributeUser) int { return len(user.Pwd) }},
		{credentialType: 3, check: func(user AttributeUser) int { return len(user.Fp) }},
		{credentialType: 4, check: func(user AttributeUser) int { return len(user.Face) }},
		{credentialType: 6, check: func(user AttributeUser) int { return len(user.Palm) }},
		{credentialType: 7, check: func(user AttributeUser) int { return len(user.Nfc) }},
	}

	for _, tt := range tests {
		pwdID, err := AddPwd(state, user.Id, tt.credentialType, "data", "", 0)
		if err != nil {
			t.Fatalf("AddPwd(type=%d) error = %v", tt.credentialType, err)
		}
		if pwdID == "" {
			t.Fatalf("AddPwd(type=%d) returned empty id", tt.credentialType)
		}
	}

	users := UsersFromReported(state.Snapshot().Reported["Users"])
	if len(users) != 1 {
		t.Fatalf("users len = %d, want 1", len(users))
	}

	for _, tt := range tests {
		if got := tt.check(users[0]); got != 1 {
			t.Fatalf("bucket for type %d count = %d, want 1", tt.credentialType, got)
		}
	}
}

func TestUpdateAndDeletePwd(t *testing.T) {
	state := New()
	user := AddUser(state, "Alice", 1)
	pwdID, err := AddPwd(state, user.Id, 2, "123456", "", 0)
	if err != nil {
		t.Fatalf("AddPwd() error = %v", err)
	}

	if err := UpdatePwd(state, user.Id, 2, pwdID, "654321", "aes", 123); err != nil {
		t.Fatalf("UpdatePwd() error = %v", err)
	}
	users := UsersFromReported(state.Snapshot().Reported["Users"])
	if got := users[0].Pwd[0]; got.Data != "654321" || got.Enc != "aes" || got.Exp != 123 {
		t.Fatalf("updated pwd = %+v", got)
	}

	if err := DelPwd(state, user.Id, 2, pwdID); err != nil {
		t.Fatalf("DelPwd() error = %v", err)
	}
	users = UsersFromReported(state.Snapshot().Reported["Users"])
	if got := len(users[0].Pwd); got != 0 {
		t.Fatalf("pwd len after delete = %d, want 0", got)
	}

	if err := UpdatePwd(state, user.Id, 2, "missing", "x", "", 0); err != ErrCredentialNotFound {
		t.Fatalf("UpdatePwd() missing error = %v, want %v", err, ErrCredentialNotFound)
	}
	if err := DelPwd(state, user.Id, 2, "missing"); err != ErrCredentialNotFound {
		t.Fatalf("DelPwd() missing error = %v, want %v", err, ErrCredentialNotFound)
	}
}

func TestUsersFromReportedSupportsGenericJSONShape(t *testing.T) {
	users := UsersFromReported([]any{
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
	})

	if len(users) != 1 {
		t.Fatalf("users len = %d, want 1", len(users))
	}
	if users[0].Id != "u1" || users[0].Name != "Alice" || len(users[0].Pwd) != 1 || users[0].Pwd[0].Id != "p1" {
		t.Fatalf("decoded users = %#v", users)
	}
}
