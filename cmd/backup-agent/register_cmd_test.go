package main

import (
	"strings"
	"testing"
)

func TestResolveRegisterToken(t *testing.T) {
	env := func(value string) func(string) string {
		return func(key string) string {
			if key == registerTokenEnv {
				return value
			}
			return ""
		}
	}

	cases := []struct {
		name, flag, envValue, want string
		wantErr                    bool
	}{
		{name: "flag only", flag: "from-flag", want: "from-flag"},
		{name: "environment only", envValue: "from-env", want: "from-env"},
		{name: "flag wins over environment", flag: "from-flag", envValue: "from-env", want: "from-flag"},
		{name: "surrounding whitespace is dropped", envValue: "  from-env\n", want: "from-env"},
		{name: "blank flag falls back to environment", flag: "   ", envValue: "from-env", want: "from-env"},
		{name: "neither given", wantErr: true},
		{name: "both blank", flag: " ", envValue: " ", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveRegisterToken(tc.flag, env(tc.envValue))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveRegisterToken() = %q, want an error", got)
				}
				if !strings.Contains(err.Error(), registerTokenEnv) {
					t.Errorf("error does not mention %s: %v", registerTokenEnv, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveRegisterToken() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveRegisterToken() = %q, want %q", got, tc.want)
			}
		})
	}
}
