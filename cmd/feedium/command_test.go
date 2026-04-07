package main

import "testing"

func TestParseCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    command
		wantErr bool
	}{
		{
			name: "default serve",
			args: nil,
			want: commandServe,
		},
		{
			name: "run serve",
			args: []string{"run"},
			want: commandServe,
		},
		{
			name: "run migrate",
			args: []string{"run", "migrate"},
			want: commandMigrate,
		},
		{
			name:    "invalid",
			args:    []string{"migrate"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseCommand(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}
