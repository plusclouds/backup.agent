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

package main

import (
	"context"

	"github.com/coreos/go-systemd/v22/dbus"
	"go.uber.org/zap"

	"github.com/plusclouds/ubuntu-agent/internal/modules/services"
)

// newServiceManager connects to systemd D-Bus and returns a Manager.
// The returned cleanup function closes the D-Bus connection; call it on shutdown.
func newServiceManager(ctx context.Context, logger *zap.Logger) (services.Manager, func()) {
	conn, err := dbus.NewSystemdConnectionContext(ctx)
	if err != nil {
		logger.Warn("could not connect to systemd D-Bus; service management unavailable",
			zap.Error(err))
		return services.New(nil, logger), func() {}
	}
	logger.Info("connected to systemd D-Bus")
	return services.New(conn, logger), func() { conn.Close() }
}
