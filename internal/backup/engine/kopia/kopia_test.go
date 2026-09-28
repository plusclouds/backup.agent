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
package kopia

import "testing"

// TestSplitEndpoint guards against the real-world bug found in manual
// testing: registration's bucket.endpoint comes back as a fully-qualified
// URL ("https://dc.s3.plusclouds.com"), but Kopia's underlying S3 client
// rejects a scheme in Endpoint ("Endpoint url cannot have fully qualified
// paths") — TLS is a separate option, not embedded in the same string.
func TestSplitEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		wantHost string
		wantTLS  bool
		wantErr  bool
	}{
		{"https URL", "https://dc.s3.plusclouds.com", "dc.s3.plusclouds.com", true, false},
		{"http URL", "http://minio.local:9000", "minio.local:9000", false, false},
		{"bare host, no scheme", "dc.s3.plusclouds.com", "dc.s3.plusclouds.com", true, false},
		{"bare host with port, no scheme", "minio.local:9000", "minio.local:9000", true, false},
		{"unsupported scheme", "ftp://example.com", "", false, true},
		{"scheme with no host", "https://", "", false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, useTLS, err := splitEndpoint(tc.endpoint)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for endpoint %q", tc.endpoint)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitEndpoint(%q): %v", tc.endpoint, err)
			}
			if host != tc.wantHost {
				t.Errorf("expected host %q, got %q", tc.wantHost, host)
			}
			if useTLS != tc.wantTLS {
				t.Errorf("expected useTLS=%v, got %v", tc.wantTLS, useTLS)
			}
		})
	}
}
