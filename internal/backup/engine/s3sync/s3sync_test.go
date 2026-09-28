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
package s3sync

import "testing"

func TestSplitEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantHost string
		wantTLS  bool
		wantErr  bool
	}{
		{name: "https URL", endpoint: "https://dc.s3.plusclouds.com", wantHost: "dc.s3.plusclouds.com", wantTLS: true},
		{name: "http URL", endpoint: "http://minio.local:9000", wantHost: "minio.local:9000", wantTLS: false},
		{name: "bare host, no scheme", endpoint: "s3.example.com", wantHost: "s3.example.com", wantTLS: true},
		{name: "unsupported scheme", endpoint: "ftp://example.com", wantErr: true},
		{name: "no host", endpoint: "https://", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, useTLS, err := splitEndpoint(tt.endpoint)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got host=%q useTLS=%v", host, useTLS)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
			if useTLS != tt.wantTLS {
				t.Errorf("useTLS = %v, want %v", useTLS, tt.wantTLS)
			}
		})
	}
}
