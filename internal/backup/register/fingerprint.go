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
