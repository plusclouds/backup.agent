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
