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
//go:build linux

package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-systemd/v22/dbus"
	"go.uber.org/zap"
)

// linuxManager implements Manager via systemd D-Bus.
type linuxManager struct {
	bus    *dbus.Conn
	logger *zap.Logger
}

// New returns a Manager backed by systemd D-Bus.
// bus may be nil — in that case every operation returns an error explaining
// that D-Bus is unavailable (graceful degradation).
func New(bus *dbus.Conn, logger *zap.Logger) Manager {
	return &linuxManager{bus: bus, logger: logger}
}

func ensureServiceSuffix(name string) string {
	if !strings.Contains(name, ".") {
		return name + ".service"
	}
	return name
}

func (m *linuxManager) List(ctx context.Context) ([]ServiceInfo, error) {
	if m.bus == nil {
		return nil, fmt.Errorf("systemd D-Bus is not available")
	}
	units, err := m.bus.ListUnitsContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing systemd units: %w", err)
	}
	services := make([]ServiceInfo, 0, len(units))
	for _, u := range units {
		if !strings.HasSuffix(u.Name, ".service") {
			continue
		}
		services = append(services, unitToServiceInfo(u))
	}
	return services, nil
}

func (m *linuxManager) Get(ctx context.Context, name string) (*ServiceInfo, error) {
	if m.bus == nil {
		return nil, fmt.Errorf("systemd D-Bus is not available")
	}
	name = ensureServiceSuffix(name)
	units, err := m.bus.ListUnitsByNamesContext(ctx, []string{name})
	if err != nil {
		return nil, fmt.Errorf("querying unit %s: %w", name, err)
	}
	if len(units) == 0 {
		return nil, fmt.Errorf("unit not found: %s", name)
	}
	info := unitToServiceInfo(units[0])
	return &info, nil
}

func (m *linuxManager) Start(ctx context.Context, name string) (*ActionResult, error) {
	if m.bus == nil {
		return failResult(name, ActionStart, fmt.Errorf("systemd D-Bus is not available")), nil
	}
	name = ensureServiceSuffix(name)
	m.logger.Info("starting service", zap.String("unit", name))
	ch := make(chan string, 1)
	if _, err := m.bus.StartUnitContext(ctx, name, "replace", ch); err != nil {
		return failResult(name, ActionStart, fmt.Errorf("start failed: %w", err)), nil
	}
	if result := <-ch; result != "done" {
		return failResult(name, ActionStart, fmt.Errorf("job result: %s", result)), nil
	}
	return &ActionResult{Service: name, Action: ActionStart, Success: true,
		Message: fmt.Sprintf("Service %s started successfully.", name)}, nil
}

func (m *linuxManager) Stop(ctx context.Context, name string) (*ActionResult, error) {
	if m.bus == nil {
		return failResult(name, ActionStop, fmt.Errorf("systemd D-Bus is not available")), nil
	}
	name = ensureServiceSuffix(name)
	m.logger.Info("stopping service", zap.String("unit", name))
	ch := make(chan string, 1)
	if _, err := m.bus.StopUnitContext(ctx, name, "replace", ch); err != nil {
		return failResult(name, ActionStop, fmt.Errorf("stop failed: %w", err)), nil
	}
	if result := <-ch; result != "done" {
		return failResult(name, ActionStop, fmt.Errorf("job result: %s", result)), nil
	}
	return &ActionResult{Service: name, Action: ActionStop, Success: true,
		Message: fmt.Sprintf("Service %s stopped successfully.", name)}, nil
}

func (m *linuxManager) Restart(ctx context.Context, name string) (*ActionResult, error) {
	if m.bus == nil {
		return failResult(name, ActionRestart, fmt.Errorf("systemd D-Bus is not available")), nil
	}
	name = ensureServiceSuffix(name)
	m.logger.Info("restarting service", zap.String("unit", name))
	ch := make(chan string, 1)
	if _, err := m.bus.RestartUnitContext(ctx, name, "replace", ch); err != nil {
		return failResult(name, ActionRestart, fmt.Errorf("restart failed: %w", err)), nil
	}
	if result := <-ch; result != "done" {
		return failResult(name, ActionRestart, fmt.Errorf("job result: %s", result)), nil
	}
	return &ActionResult{Service: name, Action: ActionRestart, Success: true,
		Message: fmt.Sprintf("Service %s restarted successfully.", name)}, nil
}

func (m *linuxManager) Enable(ctx context.Context, name string) (*ActionResult, error) {
	if m.bus == nil {
		return failResult(name, ActionEnable, fmt.Errorf("systemd D-Bus is not available")), nil
	}
	name = ensureServiceSuffix(name)
	m.logger.Info("enabling service", zap.String("unit", name))
	if _, _, err := m.bus.EnableUnitFilesContext(ctx, []string{name}, false, true); err != nil {
		return failResult(name, ActionEnable, fmt.Errorf("enable failed: %w", err)), nil
	}
	if err := m.bus.ReloadContext(ctx); err != nil {
		m.logger.Warn("daemon-reload after enable failed", zap.Error(err))
	}
	return &ActionResult{Service: name, Action: ActionEnable, Success: true,
		Message: fmt.Sprintf("Service %s enabled successfully.", name)}, nil
}

func (m *linuxManager) Disable(ctx context.Context, name string) (*ActionResult, error) {
	if m.bus == nil {
		return failResult(name, ActionDisable, fmt.Errorf("systemd D-Bus is not available")), nil
	}
	name = ensureServiceSuffix(name)
	m.logger.Info("disabling service", zap.String("unit", name))
	if _, err := m.bus.DisableUnitFilesContext(ctx, []string{name}, false); err != nil {
		return failResult(name, ActionDisable, fmt.Errorf("disable failed: %w", err)), nil
	}
	if err := m.bus.ReloadContext(ctx); err != nil {
		m.logger.Warn("daemon-reload after disable failed", zap.Error(err))
	}
	return &ActionResult{Service: name, Action: ActionDisable, Success: true,
		Message: fmt.Sprintf("Service %s disabled successfully.", name)}, nil
}

func (m *linuxManager) Reload(ctx context.Context, name string) (*ActionResult, error) {
	if m.bus == nil {
		return failResult(name, ActionReload, fmt.Errorf("systemd D-Bus is not available")), nil
	}
	name = ensureServiceSuffix(name)
	m.logger.Info("reloading service", zap.String("unit", name))
	ch := make(chan string, 1)
	if _, err := m.bus.ReloadUnitContext(ctx, name, "replace", ch); err != nil {
		return failResult(name, ActionReload, fmt.Errorf("reload failed: %w", err)), nil
	}
	if result := <-ch; result != "done" {
		return failResult(name, ActionReload, fmt.Errorf("job result: %s", result)), nil
	}
	return &ActionResult{Service: name, Action: ActionReload, Success: true,
		Message: fmt.Sprintf("Service %s reloaded successfully.", name)}, nil
}

func unitToServiceInfo(unit dbus.UnitStatus) ServiceInfo {
	state := StateUnknown
	switch unit.ActiveState {
	case "active":
		state = StateActive
	case "inactive":
		state = StateInactive
	case "failed":
		state = StateFailed
	}
	return ServiceInfo{
		Name:        unit.Name,
		Description: unit.Description,
		State:       state,
		SubState:    unit.SubState,
	}
}

func failResult(name string, action ServiceAction, err error) *ActionResult {
	return &ActionResult{Service: name, Action: action, Success: false, Message: err.Error()}
}
