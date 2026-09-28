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
// Package publisher runs backup.agent's heartbeat/telemetry loops and emits
// the job_run/alert events over NATS, using agent_type "backup".
package publisher

import (
	"context"
	"time"

	"go.uber.org/zap"

	backupconfig "github.com/plusclouds/ubuntu-agent/internal/backup/config"
	"github.com/plusclouds/ubuntu-agent/internal/backup/registry"
	"github.com/plusclouds/ubuntu-agent/internal/modules/system"
	natsclient "github.com/plusclouds/ubuntu-agent/internal/nats"
	"github.com/plusclouds/ubuntu-agent/internal/protocol"
)

const agentType = "backup"

// Publisher publishes periodic events (heartbeat, telemetry) and one-off
// events (job_run, alert) to the platform via the NATS evt subject.
type Publisher struct {
	nats      *natsclient.Client
	sys       *system.Module
	registry  *registry.Registry
	agentUUID string
	cfg       backupconfig.AgentConfig
	logger    *zap.Logger
	startTime time.Time
}

// New creates a Publisher. reg is used to report tasks_queued (the number of
// currently in-flight job runs) in every heartbeat.
func New(
	nats *natsclient.Client,
	sys *system.Module,
	reg *registry.Registry,
	agentUUID string,
	cfg backupconfig.AgentConfig,
	logger *zap.Logger,
) *Publisher {
	return &Publisher{
		nats:      nats,
		sys:       sys,
		registry:  reg,
		agentUUID: agentUUID,
		cfg:       cfg,
		logger:    logger,
		startTime: time.Now(),
	}
}

// Start launches the heartbeat and telemetry goroutines. They run until ctx
// is cancelled.
func (p *Publisher) Start(ctx context.Context) {
	go p.heartbeatLoop(ctx)
	go p.telemetryLoop(ctx)
}

func (p *Publisher) heartbeatLoop(ctx context.Context) {
	interval := p.cfg.HeartbeatInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	p.sendHeartbeat()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.sendHeartbeat()
		}
	}
}

func (p *Publisher) telemetryLoop(ctx context.Context) {
	interval := p.cfg.TelemetryInterval
	if interval <= 0 {
		interval = 60 * time.Second
	}

	p.sendTelemetry(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.sendTelemetry(ctx)
		}
	}
}

func (p *Publisher) sendHeartbeat() {
	env, err := protocol.NewForAgentType(p.agentUUID, agentType, protocol.TypeHeartbeat, protocol.HeartbeatPayload{
		Version:     backupconfig.AgentVersion,
		UptimeS:     int64(time.Since(p.startTime).Seconds()),
		TasksQueued: p.registry.RunningCount(),
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

// sendTelemetry publishes loose OS-level metrics (disk free etc). There's no
// dedicated schema for backup.agent telemetry per docs/backup.agent/protocol.md
// — the thing that actually matters (did the backup work) travels in job_run
// and result instead.
func (p *Publisher) sendTelemetry(ctx context.Context) {
	disk, err := p.sys.GetDisk(ctx)
	if err != nil {
		p.logger.Warn("telemetry: could not get disk stats", zap.Error(err))
		return
	}

	env, err := protocol.NewForAgentType(p.agentUUID, agentType, protocol.TypeTelemetry, disk)
	if err != nil {
		p.logger.Warn("telemetry: could not build envelope", zap.Error(err))
		return
	}
	if err := p.nats.Publish(env); err != nil {
		p.logger.Warn("telemetry: publish failed", zap.Error(err))
		return
	}
	p.logger.Debug("telemetry published")
}

// PublishJobRun emits a job_run event — the core message of this protocol,
// sent when the agent's own cron fires a job and it finishes. Implements
// scheduler.EventPublisher.
func (p *Publisher) PublishJobRun(_ context.Context, payload protocol.JobRunPayload) error {
	env, err := protocol.NewForAgentType(p.agentUUID, agentType, protocol.TypeJobRun, payload)
	if err != nil {
		return err
	}
	return p.nats.Publish(env)
}

// PublishAlert emits an alert event — used e.g. when the Kopia repository
// can't be reached at startup.
func (p *Publisher) PublishAlert(_ context.Context, payload protocol.AlertPayload) error {
	env, err := protocol.NewForAgentType(p.agentUUID, agentType, protocol.TypeAlert, payload)
	if err != nil {
		return err
	}
	return p.nats.Publish(env)
}
