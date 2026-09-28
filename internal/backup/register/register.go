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
// Package register implements backup.agent's one-time registration handshake
// — the one genuinely new piece of this agent type (see
// docs/backup.agent/registration.md): a token issued out-of-band by the
// platform is exchanged, once, for the agent's entire bootstrap
// configuration (NATS credentials, dedicated bucket, scoped access key, and
// initial job list).
package register

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/plusclouds/ubuntu-agent/internal/backup/state"
)

// Request is the body sent to POST {endpoint}/backup-agents/register.
type Request struct {
	Token              string `json:"token"`
	Hostname           string `json:"hostname"`
	OS                 string `json:"os"`
	Arch               string `json:"arch"`
	MachineFingerprint string `json:"machine_fingerprint"`
	AgentVersion       string `json:"agent_version"`
}

// BootstrapResponse is the platform's response to a successful registration
// — the agent's entire bootstrap config. agent_api_key and access_key.secret_key
// are shown exactly once; losing them means registering again with a new token.
type BootstrapResponse struct {
	AgentUUID   string `json:"agent_uuid"`
	AgentAPIKey string `json:"agent_api_key"`
	NATS        struct {
		Host string  `json:"host"`
		Port FlexInt `json:"port"`
	} `json:"nats"`
	Bucket struct {
		Name     string `json:"name"`
		Endpoint string `json:"endpoint"`
	} `json:"bucket"`
	AccessKey struct {
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
	} `json:"access_key"`
	Jobs struct {
		Jobs []state.Job `json:"jobs"`
	} `json:"jobs"`
	// SyncEngine, if the platform specifies one, becomes the agent's default
	// backup engine ("rsync" or "kopia" — see backupconfig.AgentConfig.SyncEngine).
	// Optional: an empty/absent value leaves whatever the local config
	// already has (an operator's explicit setting, or the "rsync" built-in
	// default) untouched — see register_cmd.go's runRegister.
	SyncEngine string `json:"sync_engine,omitempty"`
}

// Register performs the one-time token -> bootstrap-config exchange against
// {endpoint}/backup-agents/register. This route sits outside the platform's
// IAM-authenticated group per registration.md — the token itself is the
// auth, so no Authorization header is sent.
//
// The raw response body is returned alongside the parsed struct — the token
// is consumed the moment this call succeeds, so callers that want to inspect
// exactly what the platform sent (e.g. to debug a field that parsed as empty)
// need it now; there's no way to re-fetch it afterward.
func Register(ctx context.Context, endpoint, token, agentVersion string) (*BootstrapResponse, []byte, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, nil, fmt.Errorf("reading hostname: %w", err)
	}
	fingerprint, err := MachineFingerprint()
	if err != nil {
		return nil, nil, fmt.Errorf("computing machine fingerprint: %w", err)
	}

	body, err := json.Marshal(Request{
		Token:              token,
		Hostname:           hostname,
		OS:                 runtime.GOOS,
		Arch:               runtime.GOARCH,
		MachineFingerprint: fingerprint,
		AgentVersion:       agentVersion,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling registration request: %w", err)
	}

	url := strings.TrimRight(endpoint, "/") + "/backup-agents/register"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("building registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("calling %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("reading registration response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, respBody, fmt.Errorf("registration failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out BootstrapResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, respBody, fmt.Errorf("parsing registration response: %w", err)
	}
	if out.AgentUUID == "" || out.AgentAPIKey == "" {
		return nil, respBody, fmt.Errorf("registration response missing agent_uuid/agent_api_key")
	}
	return &out, respBody, nil
}
