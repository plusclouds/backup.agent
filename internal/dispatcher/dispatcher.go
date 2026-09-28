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
// Package dispatcher routes inbound command envelopes to module operations
// and returns result envelopes. Operations must be listed in the agent config's
// allowed_operations; unlisted or unknown operations are rejected without execution.
package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/plusclouds/ubuntu-agent/internal/executor"
	"github.com/plusclouds/ubuntu-agent/internal/modules/services"
	"github.com/plusclouds/ubuntu-agent/internal/modules/system"
	"github.com/plusclouds/ubuntu-agent/internal/protocol"
	"github.com/plusclouds/ubuntu-agent/internal/publisher"
)

// Dispatcher routes command envelopes to the appropriate module method.
type Dispatcher struct {
	sys             *system.Module
	svc             services.Manager
	exec            *executor.Executor
	pub             *publisher.Publisher
	allowedOps      map[string]bool
	allowedCommands []string
	agentUUID       string
	logger          *zap.Logger
}

// New creates a Dispatcher. allowedOps and allowedCommands come from agent config.
func New(
	sys *system.Module,
	svc services.Manager,
	exec *executor.Executor,
	pub *publisher.Publisher,
	agentUUID string,
	allowedOps []string,
	allowedCommands []string,
	logger *zap.Logger,
) *Dispatcher {
	ops := make(map[string]bool, len(allowedOps))
	for _, op := range allowedOps {
		ops[op] = true
	}
	return &Dispatcher{
		sys:             sys,
		svc:             svc,
		exec:            exec,
		pub:             pub,
		allowedOps:      ops,
		allowedCommands: allowedCommands,
		agentUUID:       agentUUID,
		logger:          logger,
	}
}

// params is the common command params shape. Fields are optional depending on operation.
type params struct {
	Name       string   `json:"name"`
	Command    string   `json:"command"`
	Args       []string `json:"args"`
	IntervalS  int      `json:"interval_s"`
}

// Dispatch handles a single command envelope and returns the result envelope.
// It always returns a valid result — errors become failed/rejected statuses.
func (d *Dispatcher) Dispatch(ctx context.Context, env protocol.Envelope) protocol.Envelope {
	start := time.Now()

	var cmd protocol.CommandPayload
	if err := env.DecodePayload(&cmd); err != nil {
		d.logger.Error("→ command received: could not decode payload",
			zap.String("command_id", env.ID),
			zap.Error(err),
		)
		return d.reject(env, "could not decode command payload: "+err.Error())
	}

	op := cmd.Operation

	d.logger.Info("→ command received",
		zap.String("command_id", env.ID),
		zap.String("operation", op),
	)
	d.logger.Debug("→ command params",
		zap.String("command_id", env.ID),
		zap.String("operation", op),
		zap.ByteString("params", cmd.Params),
	)

	if !d.allowedOps[op] {
		result := d.reject(env, fmt.Sprintf("operation %q is not permitted on this agent", op))
		d.logResult(env.ID, op, protocol.StatusRejected, "operation not in allowed_operations", time.Since(start))
		return result
	}

	var p params
	if len(cmd.Params) > 0 && !emptyParams(cmd.Params) {
		if err := json.Unmarshal(cmd.Params, &p); err != nil {
			result := d.reject(env, "could not decode params: "+err.Error())
			d.logResult(env.ID, op, protocol.StatusRejected, "could not decode params: "+err.Error(), time.Since(start))
			return result
		}
	}

	output, err := d.run(ctx, op, p)
	if err != nil {
		result := d.fail(env, err.Error())
		d.logResult(env.ID, op, protocol.StatusFailed, err.Error(), time.Since(start))
		return result
	}

	d.logger.Debug("← command output",
		zap.String("command_id", env.ID),
		zap.String("operation", op),
		zap.Any("output", output),
	)
	result := d.ok(env, output)
	d.logResult(env.ID, op, protocol.StatusCompleted, "", time.Since(start))
	return result
}

// logResult emits the summary log line for a completed dispatch.
func (d *Dispatcher) logResult(commandID, op, status, msg string, elapsed time.Duration) {
	fields := []zap.Field{
		zap.String("command_id", commandID),
		zap.String("operation", op),
		zap.String("status", status),
		zap.Duration("elapsed", elapsed),
	}
	if msg != "" {
		fields = append(fields, zap.String("message", msg))
	}
	switch status {
	case protocol.StatusCompleted:
		d.logger.Info("← result: completed", fields...)
	case protocol.StatusFailed:
		d.logger.Error("← result: failed", fields...)
	case protocol.StatusRejected:
		d.logger.Warn("← result: rejected", fields...)
	}
}

// run executes the operation and returns the output or an error.
func (d *Dispatcher) run(ctx context.Context, op string, p params) (any, error) {
	switch op {
	// ---- system -------------------------------------------------------
	case "system.info":
		return d.sys.GetInfo(ctx)
	case "system.metrics":
		return d.sys.GetMetrics(ctx)
	case "system.cpu":
		return d.sys.GetCPU(ctx)
	case "system.memory":
		return d.sys.GetMemory(ctx)
	case "system.disk":
		return d.sys.GetDisk(ctx)
	case "system.network":
		return d.sys.GetNetwork(ctx)

	// ---- services -----------------------------------------------------
	case "services.list":
		return d.svc.List(ctx)
	case "services.get":
		return d.svc.Get(ctx, p.Name)
	case "services.start":
		return d.svc.Start(ctx, p.Name)
	case "services.stop":
		return d.svc.Stop(ctx, p.Name)
	case "services.restart":
		return d.svc.Restart(ctx, p.Name)
	case "services.reload":
		return d.svc.Reload(ctx, p.Name)
	case "services.enable":
		return d.svc.Enable(ctx, p.Name)
	case "services.disable":
		return d.svc.Disable(ctx, p.Name)

	// ---- vm -----------------------------------------------------------
	case "vm.reboot":
		var stdout, stderr string
		var execErr error
		if runtime.GOOS == "windows" {
			stdout, stderr, execErr = d.exec.Execute(ctx, "shutdown", "/r", "/t", "0")
		} else {
			stdout, stderr, execErr = d.exec.Execute(ctx, "systemctl", "reboot")
		}
		return map[string]string{"stdout": stdout, "stderr": stderr}, execErr
	case "vm.shutdown":
		var stdout, stderr string
		var execErr error
		if runtime.GOOS == "windows" {
			stdout, stderr, execErr = d.exec.Execute(ctx, "shutdown", "/s", "/t", "0")
		} else {
			stdout, stderr, execErr = d.exec.Execute(ctx, "systemctl", "poweroff")
		}
		return map[string]string{"stdout": stdout, "stderr": stderr}, execErr

	// ---- system.update ------------------------------------------------
	case "system.update":
		distro, err := detectDebianDistro()
		if err != nil {
			return nil, fmt.Errorf("could not detect OS: %w", err)
		}
		if distro == "" {
			return nil, fmt.Errorf("system.update is only supported on Ubuntu/Debian; detected OS is not apt-based")
		}

		// Run apt-get update first, then upgrade. Stop on first failure.
		updateOut, updateErr, err := d.exec.Execute(ctx,
			"apt-get", "-y", "-o", "Dpkg::Options::=--force-confdef",
			"-o", "Dpkg::Options::=--force-confold", "update")
		if err != nil {
			return map[string]any{
				"step":   "apt-get update",
				"stdout": updateOut,
				"stderr": updateErr,
			}, fmt.Errorf("apt-get update failed: %w", err)
		}

		upgradeOut, upgradeErr, err := d.exec.Execute(ctx,
			"apt-get", "-y", "-o", "Dpkg::Options::=--force-confdef",
			"-o", "Dpkg::Options::=--force-confold", "upgrade")
		return map[string]any{
			"distro":         distro,
			"update_stdout":  updateOut,
			"update_stderr":  updateErr,
			"upgrade_stdout": upgradeOut,
			"upgrade_stderr": upgradeErr,
		}, err

	// ---- agent --------------------------------------------------------
	case "agent.allowed_operations":
		payload, err := d.pub.SendCapabilities(ctx)
		if err != nil {
			return nil, fmt.Errorf("re-publishing capabilities: %w", err)
		}
		return payload, nil

	// ---- telemetry ----------------------------------------------------
	case "telemetry.set_interval":
		if p.IntervalS <= 0 {
			return nil, fmt.Errorf("interval_s must be a positive integer (seconds)")
		}
		requested := time.Duration(p.IntervalS) * time.Second
		applied := d.pub.SetTelemetryInterval(requested)
		return map[string]any{
			"requested_interval_s": p.IntervalS,
			"applied_interval_s":   int(applied.Seconds()),
		}, nil

	// ---- exec ---------------------------------------------------------
	case "exec":
		if !slices.Contains(d.allowedCommands, p.Command) {
			return nil, fmt.Errorf("command %q is not in allowed_commands", p.Command)
		}
		stdout, stderr, err := d.exec.Execute(ctx, p.Command, p.Args...)
		return map[string]string{"stdout": stdout, "stderr": stderr}, err

	default:
		return nil, fmt.Errorf("unknown operation %q", op)
	}
}

// --- result helpers -----------------------------------------------------------

func (d *Dispatcher) ok(src protocol.Envelope, output any) protocol.Envelope {
	env, err := protocol.ResultFor(src, d.agentUUID, protocol.StatusCompleted, "", output)
	if err != nil {
		d.logger.Error("failed to build result envelope", zap.Error(err))
	}
	return env
}

func (d *Dispatcher) fail(src protocol.Envelope, msg string) protocol.Envelope {
	env, err := protocol.ResultFor(src, d.agentUUID, protocol.StatusFailed, msg, nil)
	if err != nil {
		d.logger.Error("failed to build result envelope", zap.Error(err))
	}
	return env
}

func (d *Dispatcher) reject(src protocol.Envelope, msg string) protocol.Envelope {
	env, err := protocol.ResultFor(src, d.agentUUID, protocol.StatusRejected, msg, nil)
	if err != nil {
		d.logger.Error("failed to build result envelope", zap.Error(err))
	}
	return env
}

// detectDebianDistro reads /etc/os-release and returns the distro name if the
// system is Ubuntu or Debian (or a Debian derivative). Returns "" for other
// distros and an error only if the file cannot be read.
func detectDebianDistro() (string, error) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", fmt.Errorf("reading /etc/os-release: %w", err)
	}

	var id, idLike string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ID=") {
			id = strings.ToLower(strings.Trim(strings.TrimPrefix(line, "ID="), `"`))
		}
		if strings.HasPrefix(line, "ID_LIKE=") {
			idLike = strings.ToLower(strings.Trim(strings.TrimPrefix(line, "ID_LIKE="), `"`))
		}
	}

	if id == "ubuntu" || id == "debian" ||
		strings.Contains(idLike, "ubuntu") || strings.Contains(idLike, "debian") {
		if id != "" {
			return id, nil
		}
		return idLike, nil
	}
	return "", nil
}

// emptyParams reports whether raw JSON represents an absence of params.
// PHP serializes empty arrays as [] and some callers send null or {}.
// All three are treated as "no params provided".
func emptyParams(raw []byte) bool {
	switch string(bytes.TrimSpace(raw)) {
	case "null", "[]", "{}", "":
		return true
	}
	return false
}
