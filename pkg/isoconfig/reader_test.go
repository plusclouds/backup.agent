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
package isoconfig_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/plusclouds/ubuntu-agent/pkg/isoconfig"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func sampleMetadata() isoconfig.VirtualMachineMetadata {
	gw := "185.255.175.254"
	return isoconfig.VirtualMachineMetadata{
		Hostname:         "enelsa-s-r-v",
		Username:         "root",
		Password:         "s3cr3t",
		VirtualMachineID: "016c6a79-dbbe-4284-9ea0-75645ede8ca3",
		VirtualDisks: isoconfig.DataList[isoconfig.VirtualDisk]{
			Data: []isoconfig.VirtualDisk{
				{DiskType: "user", DeviceNumber: 0, TotalDisk: 85899345920},
				{DiskType: "cdrom", DeviceNumber: 3, TotalDisk: 0},
			},
		},
		VirtualNetworkCards: isoconfig.DataList[isoconfig.VirtualNetworkCard]{
			Data: []isoconfig.VirtualNetworkCard{
				{
					DeviceNumber: 0,
					MACAddr:      "7a:9c:c0:d0:ff:bc",
					Network: isoconfig.DataWrapper[isoconfig.Network]{
						Data: isoconfig.Network{
							IPAddr:         "185.255.172.0/22",
							Gateway:        &gw,
							Subnet:         "22",
							Netmask:        "255.255.252.0",
							NetworkAddress: "185.255.172.0",
							DNSNameservers: []string{"8.8.4.4/32", "8.8.8.8/32"},
							MTU:            1500,
						},
					},
					IPList: isoconfig.DataList[isoconfig.IPEntry]{
						Data: []isoconfig.IPEntry{
							{
								ID:     "c8774f6f-1d5e-4359-b450-b0ccba1bdb01",
								IPAddr: "185.255.172.129/32",
							},
						},
					},
				},
			},
		},
		ServiceRoles:  isoconfig.DataList[isoconfig.ServiceRole]{},
		SSHPublicKeys: []string{},
	}
}

func writeYAML(t *testing.T, dir, filename string, v interface{}) {
	t.Helper()
	data, err := yaml.Marshal(v)
	if err != nil {
		t.Fatalf("yaml.Marshal %s: %v", filename, err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0600); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
}

func writeJSONFile(t *testing.T, dir, filename string, v interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal %s: %v", filename, err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0600); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
}

// ---------------------------------------------------------------------------
// Reader.Read — YAML
// ---------------------------------------------------------------------------

func TestRead_YAML_ParsesAllFields(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "metadata.yaml", sampleMetadata())

	meta, err := isoconfig.NewReader(dir).Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}

	raw := meta.Raw()
	if raw == nil {
		t.Fatal("Raw() should not be nil after reading a valid metadata.yaml")
	}
	if raw.Hostname != "enelsa-s-r-v" {
		t.Errorf("Hostname: got %q", raw.Hostname)
	}
	if raw.VirtualMachineID != "016c6a79-dbbe-4284-9ea0-75645ede8ca3" {
		t.Errorf("VirtualMachineID: got %q", raw.VirtualMachineID)
	}
	if len(raw.VirtualDisks.Data) != 2 {
		t.Errorf("VirtualDisks: expected 2, got %d", len(raw.VirtualDisks.Data))
	}
	if len(raw.VirtualNetworkCards.Data) != 1 {
		t.Errorf("VirtualNetworkCards: expected 1, got %d", len(raw.VirtualNetworkCards.Data))
	}
	nic := raw.VirtualNetworkCards.Data[0]
	if nic.MACAddr != "7a:9c:c0:d0:ff:bc" {
		t.Errorf("MACAddr: got %q", nic.MACAddr)
	}
	if len(nic.IPList.Data) != 1 {
		t.Errorf("IPList: expected 1, got %d", len(nic.IPList.Data))
	}
	if nic.IPList.Data[0].IPAddr != "185.255.172.129/32" {
		t.Errorf("IPAddr: got %q", nic.IPList.Data[0].IPAddr)
	}
}

func TestRead_YAML_InvalidYAML_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "metadata.yaml"), []byte(":\tinvalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := isoconfig.NewReader(dir).Read()
	if err == nil {
		t.Error("expected parse error for invalid YAML")
	}
}

// ---------------------------------------------------------------------------
// Reader.Read — JSON fallback
// ---------------------------------------------------------------------------

func TestRead_JSONFallback(t *testing.T) {
	dir := t.TempDir()
	writeJSONFile(t, dir, "metadata.json", sampleMetadata())

	meta, err := isoconfig.NewReader(dir).Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if meta.VMID() != "016c6a79-dbbe-4284-9ea0-75645ede8ca3" {
		t.Errorf("VMID: got %q", meta.VMID())
	}
}

func TestRead_YAMLTakesPrecedenceOverJSON(t *testing.T) {
	dir := t.TempDir()
	yaml := sampleMetadata()
	json := sampleMetadata()
	json.Hostname = "from-json"
	writeYAML(t, dir, "metadata.yaml", yaml)
	writeJSONFile(t, dir, "metadata.json", json)

	meta, err := isoconfig.NewReader(dir).Read()
	if err != nil {
		t.Fatal(err)
	}
	if meta.Hostname() == "from-json" {
		t.Error("expected YAML to take precedence over JSON fallback")
	}
	if meta.Hostname() != "enelsa-s-r-v" {
		t.Errorf("Hostname: got %q", meta.Hostname())
	}
}

func TestRead_NoFiles_ReturnsEmptyMeta(t *testing.T) {
	dir := t.TempDir()
	meta, err := isoconfig.NewReader(dir).Read()
	if err != nil {
		t.Fatalf("Read() on empty dir should not error: %v", err)
	}
	if meta.Raw() != nil {
		t.Error("Raw() should be nil when no metadata file is present")
	}
}

func TestRead_NonExistentMountPath_ReturnsEmptyMeta(t *testing.T) {
	meta, err := isoconfig.NewReader("/nonexistent/path/xyz").Read()
	if err != nil {
		t.Fatalf("Read() on missing mount path should not error: %v", err)
	}
	if meta.Raw() != nil {
		t.Error("expected nil Raw() for missing mount path")
	}
}

// ---------------------------------------------------------------------------
// ISOMetadata accessors
// ---------------------------------------------------------------------------

func readSample(t *testing.T) *isoconfig.ISOMetadata {
	t.Helper()
	dir := t.TempDir()
	writeYAML(t, dir, "metadata.yaml", sampleMetadata())
	meta, err := isoconfig.NewReader(dir).Read()
	if err != nil {
		t.Fatal(err)
	}
	return meta
}

func TestAccessors_VMID(t *testing.T) {
	if got := readSample(t).VMID(); got != "016c6a79-dbbe-4284-9ea0-75645ede8ca3" {
		t.Errorf("VMID: got %q", got)
	}
}

func TestAccessors_Hostname(t *testing.T) {
	if got := readSample(t).Hostname(); got != "enelsa-s-r-v" {
		t.Errorf("Hostname: got %q", got)
	}
}

func TestAccessors_Password(t *testing.T) {
	if got := readSample(t).Password(); got != "s3cr3t" {
		t.Errorf("Password: got %q", got)
	}
}

func TestAccessors_APIKey_EqualsPassword(t *testing.T) {
	m := readSample(t)
	if m.APIKey() != m.Password() {
		t.Errorf("APIKey should equal Password: %q != %q", m.APIKey(), m.Password())
	}
}

func TestAccessors_PrimaryIP(t *testing.T) {
	if got := readSample(t).PrimaryIP(); got != "185.255.172.129/32" {
		t.Errorf("PrimaryIP: got %q", got)
	}
}

func TestAccessors_Gateway(t *testing.T) {
	if got := readSample(t).Gateway(); got != "185.255.175.254" {
		t.Errorf("Gateway: got %q", got)
	}
}

func TestAccessors_DNSNameservers(t *testing.T) {
	dns := readSample(t).DNSNameservers()
	if len(dns) != 2 {
		t.Fatalf("DNSNameservers: expected 2, got %d", len(dns))
	}
	if dns[0] != "8.8.4.4/32" || dns[1] != "8.8.8.8/32" {
		t.Errorf("DNSNameservers: got %v", dns)
	}
}

func TestAccessors_GatewayNull_ReturnsEmpty(t *testing.T) {
	m := sampleMetadata()
	m.VirtualNetworkCards.Data[0].Network.Data.Gateway = nil
	dir := t.TempDir()
	writeYAML(t, dir, "metadata.yaml", m)
	meta, _ := isoconfig.NewReader(dir).Read()
	if got := meta.Gateway(); got != "" {
		t.Errorf("Gateway with null: expected empty, got %q", got)
	}
}

func TestAccessors_NilRaw_AllReturnZero(t *testing.T) {
	meta := &isoconfig.ISOMetadata{}
	if got := meta.VMID(); got != "" {
		t.Errorf("VMID: got %q", got)
	}
	if got := meta.Hostname(); got != "" {
		t.Errorf("Hostname: got %q", got)
	}
	if got := meta.Password(); got != "" {
		t.Errorf("Password: got %q", got)
	}
	if got := meta.APIKey(); got != "" {
		t.Errorf("APIKey: got %q", got)
	}
	if got := meta.PrimaryIP(); got != "" {
		t.Errorf("PrimaryIP: got %q", got)
	}
	if got := meta.Gateway(); got != "" {
		t.Errorf("Gateway: got %q", got)
	}
	if got := meta.TenantID(); got != "" {
		t.Errorf("TenantID: got %q", got)
	}
	if got := meta.AgentToken(); got != "" {
		t.Errorf("AgentToken: got %q", got)
	}
	if got := meta.ControlPlaneURL(); got != "" {
		t.Errorf("ControlPlaneURL: got %q", got)
	}
}

func TestAccessors_NoPrimaryIP_WhenNoNICs(t *testing.T) {
	m := sampleMetadata()
	m.VirtualNetworkCards.Data = nil
	dir := t.TempDir()
	writeYAML(t, dir, "metadata.yaml", m)
	meta, _ := isoconfig.NewReader(dir).Read()
	if got := meta.PrimaryIP(); got != "" {
		t.Errorf("PrimaryIP with no NICs: got %q", got)
	}
}
