package main

import "testing"

func TestParseCLIArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		args         []string
		wantCommand  string
		wantFlagArgs []string
	}{
		{
			name:         "default run on empty args",
			args:         nil,
			wantCommand:  "run",
			wantFlagArgs: nil,
		},
		{
			name:         "explicit run command",
			args:         []string{"run", "--config", "dev.yaml"},
			wantCommand:  "run",
			wantFlagArgs: []string{"--config", "dev.yaml"},
		},
		{
			name:         "bind command",
			args:         []string{"bind", "--config", "dev.yaml"},
			wantCommand:  "bind",
			wantFlagArgs: []string{"--config", "dev.yaml"},
		},
		{
			name:         "default run when args start with flags",
			args:         []string{"--config", "dev.yaml"},
			wantCommand:  "run",
			wantFlagArgs: []string{"--config", "dev.yaml"},
		},
		{
			name:         "unknown command stays unknown",
			args:         []string{"debug", "--config", "dev.yaml"},
			wantCommand:  "debug",
			wantFlagArgs: []string{"--config", "dev.yaml"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotCommand, gotFlagArgs := parseCLIArgs(tt.args)
			if gotCommand != tt.wantCommand {
				t.Fatalf("command = %q, want %q", gotCommand, tt.wantCommand)
			}
			if len(gotFlagArgs) != len(tt.wantFlagArgs) {
				t.Fatalf("flag args len = %d, want %d", len(gotFlagArgs), len(tt.wantFlagArgs))
			}
			for i := range gotFlagArgs {
				if gotFlagArgs[i] != tt.wantFlagArgs[i] {
					t.Fatalf("flag args[%d] = %q, want %q", i, gotFlagArgs[i], tt.wantFlagArgs[i])
				}
			}
		})
	}
}
