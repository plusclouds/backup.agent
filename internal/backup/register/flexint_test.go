// MIT License
//
// Copyright (c) 2026 PlusClouds
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
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
