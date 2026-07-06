package register

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"sort"
)

// MachineFingerprint derives a stable identifier for this machine from its
// hostname and non-loopback hardware addresses, using only the standard
// library so it behaves identically on Linux, Windows, and macOS with no new
// dependency (registration.md doesn't specify a derivation — this is a
// deliberate v1 choice).
func MachineFingerprint() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("reading hostname: %w", err)
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("listing network interfaces: %w", err)
	}

	var macs []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addr := iface.HardwareAddr.String()
		if addr == "" || addr == "00:00:00:00:00:00" {
			continue
		}
		macs = append(macs, addr)
	}
	sort.Strings(macs)

	h := sha256.New()
	h.Write([]byte(hostname)) //nolint:errcheck — hash.Hash.Write never errors
	for _, m := range macs {
		h.Write([]byte(m)) //nolint:errcheck
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
