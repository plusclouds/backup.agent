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
