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
// Package services provides types and logic for managing systemd services
// via the D-Bus system bus using go-systemd.
package services

// ServiceState represents the active state of a systemd unit.
type ServiceState string

const (
	// StateActive means the unit is running normally.
	StateActive ServiceState = "active"
	// StateInactive means the unit is not currently running.
	StateInactive ServiceState = "inactive"
	// StateFailed means the unit encountered an error and is in the failed state.
	StateFailed ServiceState = "failed"
	// StateUnknown means the unit state could not be determined.
	StateUnknown ServiceState = "unknown"
)

// ServiceInfo holds the current status of a systemd unit.
type ServiceInfo struct {
	// Name is the full systemd unit name (e.g. "nginx.service").
	Name string `json:"name"`
	// Description is the human-readable description from the unit file.
	Description string `json:"description"`
	// State is the active state of the unit.
	State ServiceState `json:"state"`
	// SubState is the more detailed sub-state string (e.g. "running", "dead", "exited").
	SubState string `json:"sub_state"`
	// Enabled indicates whether the unit is enabled to start on boot.
	Enabled bool `json:"enabled"`
	// PID is the main process ID of the running service (0 if not running).
	PID uint32 `json:"pid,omitempty"`
	// Since is the Unix timestamp (seconds) when the current state was entered.
	Since int64 `json:"since,omitempty"`
}

// ServiceAction represents an operation to perform on a systemd unit.
type ServiceAction string

const (
	// ActionStart starts a stopped unit.
	ActionStart ServiceAction = "start"
	// ActionStop stops a running unit.
	ActionStop ServiceAction = "stop"
	// ActionRestart restarts a unit (stops then starts).
	ActionRestart ServiceAction = "restart"
	// ActionEnable enables a unit to start on boot.
	ActionEnable ServiceAction = "enable"
	// ActionDisable disables a unit from starting on boot.
	ActionDisable ServiceAction = "disable"
	// ActionReload sends a reload signal to the running unit process.
	ActionReload ServiceAction = "reload"
)

// ActionResult is the outcome of performing a ServiceAction on a unit.
type ActionResult struct {
	// Service is the name of the unit the action was performed on.
	Service string `json:"service"`
	// Action is the action that was attempted.
	Action ServiceAction `json:"action"`
	// Success indicates whether the action completed without error.
	Success bool `json:"success"`
	// Message provides additional context (error description or confirmation).
	Message string `json:"message"`
}
