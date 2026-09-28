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
//go:build windows

package services

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// windowsManager is a stub that reports all service operations as unsupported.
// Full Windows SCM integration (golang.org/x/sys/windows/svc) can replace
// this stub when Windows service management is required.
type windowsManager struct {
	logger *zap.Logger
}

// New returns a stub Manager for Windows.
func New(logger *zap.Logger) Manager {
	logger.Info("service management: Windows SCM not yet implemented — service operations will return unsupported")
	return &windowsManager{logger: logger}
}

var errNotSupported = fmt.Errorf("service management is not yet supported on Windows")

func (m *windowsManager) List(_ context.Context) ([]ServiceInfo, error) {
	return nil, errNotSupported
}
func (m *windowsManager) Get(_ context.Context, _ string) (*ServiceInfo, error) {
	return nil, errNotSupported
}
func (m *windowsManager) Start(_ context.Context, name string) (*ActionResult, error) {
	return &ActionResult{Service: name, Action: ActionStart, Success: false, Message: errNotSupported.Error()}, nil
}
func (m *windowsManager) Stop(_ context.Context, name string) (*ActionResult, error) {
	return &ActionResult{Service: name, Action: ActionStop, Success: false, Message: errNotSupported.Error()}, nil
}
func (m *windowsManager) Restart(_ context.Context, name string) (*ActionResult, error) {
	return &ActionResult{Service: name, Action: ActionRestart, Success: false, Message: errNotSupported.Error()}, nil
}
func (m *windowsManager) Reload(_ context.Context, name string) (*ActionResult, error) {
	return &ActionResult{Service: name, Action: ActionReload, Success: false, Message: errNotSupported.Error()}, nil
}
func (m *windowsManager) Enable(_ context.Context, name string) (*ActionResult, error) {
	return &ActionResult{Service: name, Action: ActionEnable, Success: false, Message: errNotSupported.Error()}, nil
}
func (m *windowsManager) Disable(_ context.Context, name string) (*ActionResult, error) {
	return &ActionResult{Service: name, Action: ActionDisable, Success: false, Message: errNotSupported.Error()}, nil
}
