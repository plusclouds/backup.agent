package register

import (
	"encoding/json"
	"testing"
)

func TestFlexIntAcceptsNumberAndString(t *testing.T) {
	type wrapper struct {
		Port FlexInt `json:"port"`
	}

	cases := []struct {
		name string
		json string
		want int
	}{
		{"bare number", `{"port": 4222}`, 4222},
		{"numeric string", `{"port": "4222"}`, 4222},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w wrapper
			if err := json.Unmarshal([]byte(tc.json), &w); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if int(w.Port) != tc.want {
				t.Errorf("expected %d, got %d", tc.want, w.Port)
			}
		})
	}
}

func TestFlexIntRejectsNonNumericString(t *testing.T) {
	type wrapper struct {
		Port FlexInt `json:"port"`
	}
	var w wrapper
	if err := json.Unmarshal([]byte(`{"port": "not-a-number"}`), &w); err == nil {
		t.Fatalf("expected an error for a non-numeric string")
	}
}
