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
