package register

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterSuccess(t *testing.T) {
	var gotReq Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backup-agents/register" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}

		resp := BootstrapResponse{
			AgentUUID:   "agent-uuid-1",
			AgentAPIKey: "api-key-1",
		}
		resp.NATS.Host = "nats.example.com"
		resp.NATS.Port = 4222
		resp.Bucket.Name = "backup-agent-abc123"
		resp.Bucket.Endpoint = "https://s3.example.com"
		resp.AccessKey.AccessKey = "AKIA..."
		resp.AccessKey.SecretKey = "secret"

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encoding response: %v", err)
		}
	}))
	defer srv.Close()

	resp, raw, err := Register(t.Context(), srv.URL, "one-time-token", "0.1.0")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if len(raw) == 0 {
		t.Errorf("expected the raw response body to be returned alongside the parsed struct")
	}

	if gotReq.Token != "one-time-token" {
		t.Errorf("expected token to be sent, got %q", gotReq.Token)
	}
	if gotReq.AgentVersion != "0.1.0" {
		t.Errorf("expected agent_version to be sent, got %q", gotReq.AgentVersion)
	}
	if gotReq.Hostname == "" {
		t.Errorf("expected hostname to be populated")
	}
	if gotReq.OS == "" || gotReq.Arch == "" {
		t.Errorf("expected os/arch to be populated")
	}

	if resp.AgentUUID != "agent-uuid-1" || resp.AgentAPIKey != "api-key-1" {
		t.Errorf("unexpected bootstrap response: %+v", resp)
	}
	if resp.Bucket.Name != "backup-agent-abc123" {
		t.Errorf("expected bucket name to round-trip, got %q", resp.Bucket.Name)
	}
}

func TestRegisterRejectsErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"code":"TOKEN_EXPIRED","message":"token has expired"}}`)) //nolint:errcheck
	}))
	defer srv.Close()

	_, raw, err := Register(t.Context(), srv.URL, "expired-token", "0.1.0")
	if err == nil {
		t.Fatalf("expected an error for a 401 response")
	}
	if len(raw) == 0 {
		t.Errorf("expected the raw error body to be returned even on failure")
	}
}

func TestRegisterRejectsMissingCredentialsInResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`)) //nolint:errcheck
	}))
	defer srv.Close()

	_, _, err := Register(t.Context(), srv.URL, "token", "0.1.0")
	if err == nil {
		t.Fatalf("expected an error when agent_uuid/agent_api_key are missing")
	}
}

func TestMachineFingerprintIsStable(t *testing.T) {
	a, err := MachineFingerprint()
	if err != nil {
		t.Fatalf("MachineFingerprint: %v", err)
	}
	b, err := MachineFingerprint()
	if err != nil {
		t.Fatalf("MachineFingerprint (second call): %v", err)
	}
	if a != b {
		t.Errorf("expected MachineFingerprint to be stable across calls, got %q vs %q", a, b)
	}
	if len(a) != 64 { // hex-encoded SHA-256
		t.Errorf("expected a 64-character hex digest, got length %d", len(a))
	}
}
