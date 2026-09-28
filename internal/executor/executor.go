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
// Package executor provides a safe, audited command execution utility.
// All commands are executed directly via os/exec (never through a shell),
// with full stdout/stderr capture and structured audit logging via zap.
package executor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"go.uber.org/zap"
)

const defaultTimeout = 30 * time.Second

// Executor runs external commands safely.
type Executor struct {
	logger  *zap.Logger
	timeout time.Duration
}

// New creates an Executor with the default 30-second timeout.
func New(logger *zap.Logger) *Executor {
	return &Executor{
		logger:  logger,
		timeout: defaultTimeout,
	}
}

// NewWithTimeout creates an Executor with the specified timeout.
func NewWithTimeout(logger *zap.Logger, timeout time.Duration) *Executor {
	return &Executor{
		logger:  logger,
		timeout: timeout,
	}
}

// Execute runs the given command with the provided arguments.
// It applies a deadline to the provided context (the shorter of the context
// deadline and the executor timeout), captures stdout and stderr separately,
// and emits a structured audit log entry on every execution.
//
// The command is always invoked directly — never via sh -c — to prevent
// shell injection.
func (e *Executor) Execute(ctx context.Context, command string, args ...string) (stdout string, stderr string, err error) {
	// Apply timeout: if the context already has a shorter deadline, that wins.
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	// Structured audit log — emitted for every execution regardless of outcome.
	e.logger.Info("command executed",
		zap.String("command", command),
		zap.Strings("args", args),
		zap.Duration("duration", duration),
		zap.Int("exit_code", exitCode),
		zap.Bool("success", runErr == nil),
	)

	if runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return stdout, stderr, fmt.Errorf("command timed out after %s: %s", e.timeout, command)
		}
		return stdout, stderr, fmt.Errorf("command failed (exit %d): %w", exitCode, runErr)
	}

	return stdout, stderr, nil
}
