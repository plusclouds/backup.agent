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
// Package publisher runs the heartbeat and telemetry loops that push events
// from the agent to the platform via the NATS evt subject.
package publisher

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/plusclouds/ubuntu-agent/internal/config"
	natsclient "github.com/plusclouds/ubuntu-agent/internal/nats"
	"github.com/plusclouds/ubuntu-agent/internal/modules/system"
	"github.com/plusclouds/ubuntu-agent/internal/protocol"
)

const minTelemetryInterval = 1 * time.Second

// Publisher publishes periodic events (heartbeat, telemetry) to the platform.
type Publisher struct {
	nats                *natsclient.Client
	sys                 *system.Module
	agentUUID           string
	cfg                 config.AgentConfig
	logger              *zap.Logger
	telemetryIntervalCh chan time.Duration
}

// New creates a Publisher.
func New(
	nats *natsclient.Client,
	sys *system.Module,
	agentUUID string,
	cfg config.AgentConfig,
	logger *zap.Logger,
) *Publisher {
	return &Publisher{
		nats:                nats,
		sys:                 sys,
		agentUUID:           agentUUID,
		cfg:                 cfg,
		logger:              logger,
		telemetryIntervalCh: make(chan time.Duration, 1),
	}
}

// Start launches the heartbeat and telemetry goroutines and announces
// capabilities. They run until ctx is cancelled.
func (p *Publisher) Start(ctx context.Context) {
	p.sendCapabilities(ctx) //nolint:errcheck — result unused on startup
	go p.heartbeatLoop(ctx)
	go p.telemetryLoop(ctx)
}

// SetTelemetryInterval updates the telemetry push interval at runtime.
// The minimum allowed value is 5s; smaller values are clamped.
func (p *Publisher) SetTelemetryInterval(d time.Duration) time.Duration {
	if d < minTelemetryInterval {
		d = minTelemetryInterval
	}
	// Non-blocking send: if a pending update is already queued, drop it and
	// replace with the newest value.
	select {
	case p.telemetryIntervalCh <- d:
	default:
		<-p.telemetryIntervalCh
		p.telemetryIntervalCh <- d
	}
	return d
}

func (p *Publisher) heartbeatLoop(ctx context.Context) {
	interval := p.cfg.HeartbeatInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	p.sendHeartbeat(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.sendHeartbeat(ctx)
		}
	}
}

func (p *Publisher) telemetryLoop(ctx context.Context) {
	interval := p.cfg.TelemetryInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	p.sendTelemetry(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case newInterval := <-p.telemetryIntervalCh:
			ticker.Reset(newInterval)
			p.logger.Info("telemetry interval updated", zap.Duration("interval", newInterval))
			// Send an immediate snapshot so the platform doesn't wait for the new tick.
			p.sendTelemetry(ctx)
		case <-ticker.C:
			p.sendTelemetry(ctx)
		}
	}
}

func (p *Publisher) sendHeartbeat(ctx context.Context) {
	info, err := p.sys.GetInfo(ctx)
	if err != nil {
		p.logger.Warn("heartbeat: could not get system info", zap.Error(err))
		return
	}

	env, err := protocol.New(p.agentUUID, protocol.TypeHeartbeat, protocol.HeartbeatPayload{
		Version: "0.2.0",
		UptimeS: info.Uptime,
	})
	if err != nil {
		p.logger.Warn("heartbeat: could not build envelope", zap.Error(err))
		return
	}

	if err := p.nats.Publish(env); err != nil {
		p.logger.Warn("heartbeat: publish failed", zap.Error(err))
		return
	}
	p.logger.Debug("heartbeat published")
}

// operationCatalog is the full schema for every supported operation.
// Only operations present in AllowedOperations are published.
var operationCatalog = map[string]protocol.OperationSchema{
	"agent.allowed_operations": {
		Operation:   "agent.allowed_operations",
		Description: "Return and re-publish the current capabilities list (allowed operations and parameters).",
	},
	"services.list": {
		Operation:   "services.list",
		Description: "List all loaded systemd services on the machine.",
	},
	"services.get": {
		Operation:   "services.get",
		Description: "Get detailed status of a single systemd service.",
		Params: []protocol.OperationParam{
			{Name: "name", Type: "string", Required: true, Description: "Service name (e.g. nginx or nginx.service)."},
		},
	},
	"services.start": {
		Operation:   "services.start",
		Description: "Start a stopped systemd service.",
		Params: []protocol.OperationParam{
			{Name: "name", Type: "string", Required: true, Description: "Service name to start."},
		},
	},
	"services.stop": {
		Operation:   "services.stop",
		Description: "Stop a running systemd service.",
		Params: []protocol.OperationParam{
			{Name: "name", Type: "string", Required: true, Description: "Service name to stop."},
		},
	},
	"services.restart": {
		Operation:   "services.restart",
		Description: "Restart a systemd service (stop then start).",
		Params: []protocol.OperationParam{
			{Name: "name", Type: "string", Required: true, Description: "Service name to restart."},
		},
	},
	"services.reload": {
		Operation:   "services.reload",
		Description: "Send a reload signal to a running service without stopping it.",
		Params: []protocol.OperationParam{
			{Name: "name", Type: "string", Required: true, Description: "Service name to reload."},
		},
	},
	"services.enable": {
		Operation:   "services.enable",
		Description: "Enable a service to start automatically on boot.",
		Params: []protocol.OperationParam{
			{Name: "name", Type: "string", Required: true, Description: "Service name to enable."},
		},
	},
	"services.disable": {
		Operation:   "services.disable",
		Description: "Disable a service from starting automatically on boot.",
		Params: []protocol.OperationParam{
			{Name: "name", Type: "string", Required: true, Description: "Service name to disable."},
		},
	},
	"system.info": {
		Operation:   "system.info",
		Description: "Return static system information (hostname, OS, kernel, uptime).",
	},
	"system.metrics": {
		Operation:   "system.metrics",
		Description: "Return a full resource snapshot (CPU, memory, disk, network).",
	},
	"system.cpu": {
		Operation:   "system.cpu",
		Description: "Return CPU usage, core count, load averages, and per-core usage.",
	},
	"system.memory": {
		Operation:   "system.memory",
		Description: "Return RAM utilisation (total, used, usage %).",
	},
	"system.disk": {
		Operation:   "system.disk",
		Description: "Return disk usage and I/O stats for all real block-device partitions.",
	},
	"system.network": {
		Operation:   "system.network",
		Description: "Return network I/O counters for all physical interfaces.",
	},
	"system.update": {
		Operation:   "system.update",
		Description: "Run apt-get update && apt-get upgrade -y. Supported on Ubuntu/Debian only.",
	},
	"telemetry.set_interval": {
		Operation:   "telemetry.set_interval",
		Description: "Change the telemetry push interval at runtime. Minimum 5 seconds.",
		Params: []protocol.OperationParam{
			{Name: "interval_s", Type: "integer", Required: true,
				Description: "New telemetry interval in seconds.", MinValue: intPtr(1)},
		},
	},
	"vm.reboot": {
		Operation:   "vm.reboot",
		Description: "Reboot the machine immediately.",
	},
	"vm.shutdown": {
		Operation:   "vm.shutdown",
		Description: "Shut down the machine immediately.",
	},
	"exec": {
		Operation:   "exec",
		Description: "Execute an allowed binary on the machine. The command must be listed in exec_commands.",
		Params: []protocol.OperationParam{
			{Name: "command", Type: "string", Required: true, Description: "Absolute path of the binary to run. Must be in the exec_commands allowlist."},
			{Name: "args", Type: "array", Required: false, Description: "List of arguments to pass to the binary."},
		},
	},
}

func intPtr(v int) *int { return &v }

// SendCapabilities re-publishes the capabilities event and returns the payload.
// Called on boot and whenever the platform sends an agent.allowed_operations command.
func (p *Publisher) SendCapabilities(ctx context.Context) (*protocol.CapabilitiesPayload, error) {
	payload := p.sendCapabilities(ctx)
	return payload, nil
}

func (p *Publisher) sendCapabilities(_ context.Context) *protocol.CapabilitiesPayload {
	p.logger.Info("capabilities: building from config",
		zap.Int("allowed_operations_in_config", len(p.cfg.AllowedOperations)),
		zap.Strings("allowed_operations", p.cfg.AllowedOperations),
	)

	// If allowed_operations is empty (config not loaded or intentionally blank),
	// fall back to publishing every operation in the catalog so the platform
	// always receives a usable list.
	source := p.cfg.AllowedOperations
	if len(source) == 0 {
		p.logger.Warn("capabilities: allowed_operations is empty — publishing full catalog as fallback")
		for op := range operationCatalog {
			source = append(source, op)
		}
	}

	schemas := make([]protocol.OperationSchema, 0, len(source))
	execAllowed := false

	for _, op := range source {
		if op == "exec" {
			execAllowed = true
		}
		if schema, ok := operationCatalog[op]; ok {
			schemas = append(schemas, schema)
		}
	}

	payload := protocol.CapabilitiesPayload{
		Operations: schemas,
	}
	if execAllowed {
		payload.ExecCommands = p.cfg.AllowedCommands
	}

	env, err := protocol.New(p.agentUUID, protocol.TypeCapabilities, payload)
	if err != nil {
		p.logger.Error("capabilities: could not build envelope", zap.Error(err))
		return &payload
	}

	if err := p.nats.Publish(env); err != nil {
		p.logger.Error("capabilities: publish failed", zap.Error(err))
		return &payload
	}

	p.logger.Info("capabilities published",
		zap.Int("operation_count", len(schemas)),
		zap.Bool("exec_allowed", execAllowed),
	)
	return &payload
}

func (p *Publisher) sendTelemetry(ctx context.Context) {
	metrics, err := p.sys.GetMetrics(ctx)
	if err != nil {
		p.logger.Warn("telemetry: could not get metrics", zap.Error(err))
		return
	}

	env, err := protocol.New(p.agentUUID, protocol.TypeTelemetry, metrics)
	if err != nil {
		p.logger.Warn("telemetry: could not build envelope", zap.Error(err))
		return
	}

	// Publish to platform evt subject.
	if err := p.nats.Publish(env); err != nil {
		p.logger.Warn("telemetry: publish to evt failed", zap.Error(err))
		return
	}

	// Also publish to the client-facing vm.{uuid}.telemetry subject (VM_TELEMETRY
	// JetStream stream, 15-minute retention). Requires the JWT to include publish
	// permission for this subject — a warning is logged if not yet granted.
	if err := p.nats.PublishToSubject(p.nats.TelemetrySubject(), env); err != nil {
		p.logger.Warn("telemetry: publish to vm telemetry subject failed",
			zap.String("subject", p.nats.TelemetrySubject()),
			zap.Error(err),
		)
	}

	p.logger.Debug("telemetry published")
}
