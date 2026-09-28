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
package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"io"

	"golang.org/x/crypto/hkdf"
)

// hkdfSalt and hkdfInfo are fixed, non-secret domain-separation strings —
// only access_key.secret_key and agent_uuid contribute entropy/uniqueness.
const hkdfSalt = "plusclouds-backup-agent/v1/kopia-repo-password"

// DeriveRepositoryPassword deterministically derives the Kopia repository
// encryption password from the agent's scoped S3 secret key and its UUID.
//
// ASSUMPTION, flagged explicitly: docs/backup.agent/registration.md's
// bootstrap response has no dedicated field for a Kopia repository
// password, so this ties repository encryption to a credential the platform
// could rotate independently of backup data. This is a v1 stopgap — a
// future registration response version should carry its own repo_password
// field instead of deriving one client-side.
func DeriveRepositoryPassword(secretKey, agentUUID string) (string, error) {
	kdf := hkdf.New(sha256.New, []byte(secretKey), []byte(hkdfSalt), []byte(agentUUID))
	out := make([]byte, 32)
	if _, err := io.ReadFull(kdf, out); err != nil {
		return "", err
	}
	return hex.EncodeToString(out), nil
}
