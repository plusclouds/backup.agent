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
// Package nats provides the NATS client used by the agent to subscribe to
// commands and publish events to the platform.
package nats

import (
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/plusclouds/ubuntu-agent/internal/config"
	"github.com/plusclouds/ubuntu-agent/internal/protocol"
)

// Client wraps a NATS connection for the agent.
type Client struct {
	conn      *nats.Conn
	agentType string
	agentUUID string
	logger    *zap.Logger
}

// Connect establishes a NATS connection authenticated with agentUUID / agentAPIKey
// and returns a ready Client stamped with agent_type "vm". Kept for backward
// compatibility with the VM agent's existing call site — new agent types
// should use ConnectForAgentType.
//
// The agent JWT issued by the NATS auth callout grants permission only to:
//   - Subscribe: agent.vm.{uuid}.cmd
//   - Publish:   agent.vm.{uuid}.evt
//
// JetStream consumer management (which requires $JS.API.* publish and _INBOX.*
// subscribe) is therefore not available to the agent. Message durability is
// handled server-side by the AGENT_COMMANDS JetStream stream; the platform
// retries unacknowledged commands via AgentCommandTimeoutCommand.
func Connect(cfg config.NATSConfig, agentUUID, agentAPIKey string, logger *zap.Logger) (*Client, error) {
	return ConnectForAgentType(cfg, "vm", agentUUID, agentAPIKey, logger)
}

// ConnectForAgentType establishes a NATS connection authenticated with
// agentUUID / agentAPIKey and returns a ready Client stamped with the given
// agent_type. The agent JWT issued by the NATS auth callout grants
// permission only to subscribe to agent.{agentType}.{uuid}.cmd and publish
// to agent.{agentType}.{uuid}.evt — same generic pattern for every agent type.
func ConnectForAgentType(cfg config.NATSConfig, agentType, agentUUID, agentAPIKey string, logger *zap.Logger) (*Client, error) {
	opts := []nats.Option{
		nats.Name("plusclouds-" + agentType + "-agent:" + agentUUID),
		nats.UserInfo(agentUUID, agentAPIKey),
		nats.MaxReconnects(cfg.MaxReconnects),
		nats.ReconnectWait(cfg.ReconnectWait),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				logger.Warn("NATS disconnected", zap.Error(err))
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("NATS reconnected", zap.String("url", nc.ConnectedUrl()))
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			logger.Info("NATS connection closed")
		}),
	}

	activeURL := cfg.ActiveURL()
	logger.Info("connecting to NATS",
		zap.String("connection_type", cfg.ConnectionType),
		zap.String("url", activeURL),
	)

	nc, err := nats.Connect(activeURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to NATS (%s) at %s: %w", cfg.ConnectionType, activeURL, err)
	}

	logger.Info("connected to NATS",
		zap.String("connection_type", cfg.ConnectionType),
		zap.String("url", nc.ConnectedUrl()),
		zap.String("agent_uuid", agentUUID),
	)

	return &Client{
		conn:      nc,
		agentType: agentType,
		agentUUID: agentUUID,
		logger:    logger,
	}, nil
}

// CmdSubject returns the subject the agent subscribes to for inbound commands.
func (c *Client) CmdSubject() string {
	return "agent." + c.agentType + "." + c.agentUUID + ".cmd"
}

// EvtSubject returns the subject the agent publishes events to.
func (c *Client) EvtSubject() string {
	return "agent." + c.agentType + "." + c.agentUUID + ".evt"
}

// TelemetrySubject returns the client-facing telemetry subject captured by
// the VM_TELEMETRY JetStream stream (15-minute retention).
func (c *Client) TelemetrySubject() string {
	return "vm." + c.agentUUID + ".telemetry"
}

// Subscribe registers a plain NATS subscription on the cmd subject.
// handler is called for each inbound command envelope.
func (c *Client) Subscribe(handler func(env protocol.Envelope)) error {
	subject := c.CmdSubject()

	_, err := c.conn.Subscribe(subject, func(msg *nats.Msg) {
		c.logger.Debug("raw message received",
			zap.String("subject", msg.Subject),
			zap.Int("bytes", len(msg.Data)),
			zap.ByteString("data", msg.Data),
		)

		var env protocol.Envelope
		if err := json.Unmarshal(msg.Data, &env); err != nil {
			c.logger.Warn("dropping unparseable command message",
				zap.String("subject", msg.Subject),
				zap.Error(err),
			)
			return
		}
		if env.V != 1 {
			c.logger.Warn("dropping command with unknown protocol version",
				zap.String("subject", msg.Subject),
				zap.Int("v", env.V),
			)
			return
		}
		handler(env)
	})
	if err != nil {
		return fmt.Errorf("subscribing to %s: %w", subject, err)
	}

	c.logger.Info("subscribed to cmd subject",
		zap.String("subject", subject),
	)
	return nil
}

// Publish marshals env to JSON and publishes it to the agent's evt subject.
func (c *Client) Publish(env protocol.Envelope) error {
	return c.PublishToSubject(c.EvtSubject(), env)
}

// PublishToSubject marshals env to JSON and publishes it to the given subject.
func (c *Client) PublishToSubject(subject string, env protocol.Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshalling envelope: %w", err)
	}
	if err := c.conn.Publish(subject, data); err != nil {
		return fmt.Errorf("publishing to %s: %w", subject, err)
	}
	return nil
}

// Drain gracefully drains pending messages and closes the connection.
func (c *Client) Drain() {
	if err := c.conn.Drain(); err != nil {
		c.logger.Warn("NATS drain error", zap.Error(err))
	}
}
