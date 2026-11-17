package dbdump

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// maxStderr bounds how much of a client tool's stderr ends up in an error
// (and therefore in a job_run event).
const maxStderr = 2000

// run executes a client tool with extra environment variables and returns
// its stdout. On failure the tool's own stderr is part of the error, so a
// failed dump says why ("password authentication failed…") rather than just
// "exit status 1". Secrets go in env, never in args.
func run(ctx context.Context, binDir, tool string, env []string, args ...string) (string, error) {
	bin, err := lookTool(binDir, tool)
	if err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("%s: %w", tool, ctxErr)
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > maxStderr {
			msg = msg[:maxStderr] + "…"
		}
		if msg == "" {
			return "", fmt.Errorf("%s: %w", tool, err)
		}
		return "", fmt.Errorf("%s: %w: %s", tool, err, msg)
	}
	return stdout.String(), nil
}

// lookTool finds tool in binDir if set; otherwise on PATH, and failing that
// in the directories the database installers use on this platform.
func lookTool(binDir, tool string) (string, error) {
	if binDir == "" {
		path, err := exec.LookPath(tool)
		if err == nil {
			return path, nil
		}
		if found := findInstalledTool(toolFile(tool)); found != "" {
			return found, nil
		}
		return "", fmt.Errorf("%s not found on PATH or in the usual install directories (install the client tools or set bin_dir for this connection): %w", tool, err)
	}

	path := filepath.Join(binDir, toolFile(tool))
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%s not found in bin_dir %s", tool, binDir)
		}
		return "", err
	}
	return path, nil
}

// toolFile is the file name a tool has on disk: "mysqldump.exe" on Windows.
func toolFile(tool string) string {
	if runtime.GOOS == "windows" && filepath.Ext(tool) == "" {
		return tool + ".exe"
	}
	return tool
}
