// Package protocol defines the NATS message envelope shared by all agent
// communication as specified in docs/protocol.md.
package protocol

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"
)

// Message type constants — used in Envelope.Type.
const (
	TypeCommand      = "command"
	TypeHeartbeat    = "heartbeat"
	TypeTelemetry    = "telemetry"
	TypeDiskHealth   = "disk_health"
	TypeAlert        = "alert"
	TypeIPMI         = "ipmi"
	TypeResult       = "result"
	TypeCapabilities = "capabilities"
	// TypeJobRun is emitted by backup.agent when its own cron scheduler fires
	// a job and it finishes. There is no originating command, so — unlike
	// TypeResult — this message carries no command_id (see docs/backup.agent/protocol.md).
	TypeJobRun = "job_run"
)

// Result status constants — used in ResultPayload.Status.
const (
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusRejected  = "rejected"
)

// Envelope is the top-level wrapper for every NATS message in both directions.
type Envelope struct {
	V         int    `json:"v"`
	ID        string `json:"id"`
	Type      string `json:"type"`
	AgentType string `json:"agent_type"`
	AgentUUID string `json:"agent_uuid"`
	Timestamp int64  `json:"timestamp"`
	// ReplyTo is set by the platform on sync commands (AgentCommandService::send).
	// The agent publishes the result directly to this inbox subject instead of
	// the evt subject. Absent on all other messages.
	ReplyTo string          `json:"reply_to,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

// CommandPayload is the payload shape for platform → agent command messages.
type CommandPayload struct {
	Operation string          `json:"operation"`
	Params    json.RawMessage `json:"params,omitempty"`
	TimeoutS  int             `json:"timeout_s,omitempty"`
}

// ResultPayload is the payload shape for agent → platform result messages.
type ResultPayload struct {
	CommandID string `json:"command_id"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	Output    any    `json:"output,omitempty"`
}

// HeartbeatPayload is the payload shape for heartbeat events.
type HeartbeatPayload struct {
	Version     string `json:"version"`
	UptimeS     int64  `json:"uptime_s"`
	TasksQueued int    `json:"tasks_queued"`
}

// AlertPayload is the payload shape for alert events, shared by every agent
// type. ObjectType/ObjectID are used by agent types that alert about a
// specific sub-resource (e.g. a disk); agent types that don't have an
// equivalent (e.g. backup.agent) leave them empty.
type AlertPayload struct {
	Severity   string `json:"severity"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	ObjectType string `json:"object_type,omitempty"`
	ObjectID   string `json:"object_id,omitempty"`
	Details    any    `json:"details,omitempty"`
}

// JobRunPayload is the payload shape for backup.agent's job_run event — sent
// when the agent's own cron scheduler fires a job and it finishes. Unlike
// ResultPayload, there is no CommandID: this message never replies to a
// platform-initiated command (see docs/backup.agent/protocol.md).
type JobRunPayload struct {
	JobUUID         string  `json:"job_uuid"`
	RunUUID         string  `json:"run_uuid"`
	Status          string  `json:"status"`
	StartedAt       string  `json:"started_at"`
	FinishedAt      string  `json:"finished_at"`
	BytesUploaded   int64   `json:"bytes_uploaded"`
	BytesDeduped    int64   `json:"bytes_deduped"`
	KopiaSnapshotID string  `json:"kopia_snapshot_id,omitempty"`
	Error           *string `json:"error"`
}

// OperationParam describes a single parameter accepted by an operation.
type OperationParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // "string", "integer", "array"
	Required    bool   `json:"required"`
	Description string `json:"description"`
	MinValue    *int   `json:"min_value,omitempty"` // for integer types
}

// OperationSchema describes one executable operation including its parameters.
type OperationSchema struct {
	Operation   string           `json:"operation"`
	Description string           `json:"description"`
	Params      []OperationParam `json:"params,omitempty"`
}

// CapabilitiesPayload is published once on agent startup so the platform knows
// which operations are currently enabled and what parameters each one accepts.
// ExecCommands is only present when the "exec" operation is enabled.
type CapabilitiesPayload struct {
	Operations   []OperationSchema `json:"operations"`
	ExecCommands []string          `json:"exec_commands,omitempty"`
}

// New returns a new Envelope with a generated UUID v4 ID and current timestamp,
// stamped with agent_type "vm". Kept for backward compatibility with the VM
// agent's existing call sites — new agent types should use NewForAgentType.
func New(agentUUID, msgType string, payload any) (Envelope, error) {
	return NewForAgentType(agentUUID, "vm", msgType, payload)
}

// NewForAgentType returns a new Envelope with a generated UUID v4 ID and
// current timestamp, stamped with the given agent_type. This is the
// agent-type-agnostic constructor every non-VM agent (e.g. backup.agent)
// should use.
func NewForAgentType(agentUUID, agentType, msgType string, payload any) (Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshalling payload: %w", err)
	}
	return Envelope{
		V:         1,
		ID:        newUUID(),
		Type:      msgType,
		AgentType: agentType,
		AgentUUID: agentUUID,
		Timestamp: time.Now().Unix(),
		Payload:   raw,
	}, nil
}

// DecodePayload unmarshals the envelope's Payload into dst.
func (e *Envelope) DecodePayload(dst any) error {
	return json.Unmarshal(e.Payload, dst)
}

// ResultFor constructs a result Envelope that echoes back the command ID from src,
// stamped with agent_type "vm". Kept for backward compatibility with the VM
// agent's existing call sites — new agent types should use ResultForType.
func ResultFor(src Envelope, agentUUID, status, message string, output any) (Envelope, error) {
	return ResultForType(src, agentUUID, "vm", status, message, output)
}

// ResultForType constructs a result Envelope that echoes back the command ID
// from src, stamped with the given agent_type.
func ResultForType(src Envelope, agentUUID, agentType, status, message string, output any) (Envelope, error) {
	return NewForAgentType(agentUUID, agentType, TypeResult, ResultPayload{
		CommandID: src.ID,
		Status:    status,
		Message:   message,
		Output:    output,
	})
}

// NewUUID generates a random UUID v4 string. Exported for callers that need
// to mint an ID outside of an Envelope (e.g. backup.agent generating its own
// run_uuid for a cron-triggered job_run event).
func NewUUID() string {
	return newUUID()
}

// newUUID generates a random UUID v4 string.
func newUUID() string {
	var b [16]byte
	rand.Read(b[:]) //nolint:errcheck — crypto/rand.Read never errors on Linux
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
