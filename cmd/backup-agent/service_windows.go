//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	backupconfig "github.com/plusclouds/ubuntu-agent/internal/backup/config"
)

const (
	windowsServiceName        = "stash"
	windowsServiceDisplayName = "PlusClouds Stash backup agent"
)

// installSystemService registers stash with the Windows Service Control
// Manager and starts it, right after a successful register — the Windows
// counterpart of the systemd unit in service_linux.go. The service runs
// `stash run --config <the file register just wrote>` as Local System,
// starts with the machine, and is restarted by Windows if it exits.
//
// Needs an elevated (administrator) shell: without one the Service Control
// Manager refuses the connection, and this prints how to start by hand
// instead. Running it again on a machine that already has the service
// updates the command line and restarts it.
func installSystemService() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("Warning: could not find this program's own path: %v\n", err)
		fmt.Println("Start manually with: stash run")
		return
	}
	configPath := cfgFile
	if configPath == "" {
		configPath = backupconfig.DefaultConfigPath
	}
	// A service starts in System32, so a relative --config would not resolve.
	if abs, err := filepath.Abs(configPath); err == nil {
		configPath = abs
	}
	args := []string{"run", "--config", configPath}

	manager, err := mgr.Connect()
	if err != nil {
		fmt.Printf("Could not reach the Windows Service Control Manager (%v) — skipping automatic service install.\n", err)
		fmt.Println("Run `stash register` from an administrator shell to have stash run as a service, or start it manually with: stash run")
		return
	}
	defer manager.Disconnect() //nolint:errcheck

	service, err := manager.OpenService(windowsServiceName)
	if err == nil {
		// Already installed (a re-register): stop it, then point it at the
		// current binary and config before starting again.
		stopWindowsService(service)
		cfg, cfgErr := service.Config()
		if cfgErr == nil {
			cfg.BinaryPathName = windowsCommandLine(exe, args)
			cfgErr = service.UpdateConfig(cfg)
		}
		if cfgErr != nil {
			service.Close() //nolint:errcheck
			fmt.Printf("Warning: could not update the existing %s service: %v\n", windowsServiceName, cfgErr)
			return
		}
	} else {
		service, err = manager.CreateService(windowsServiceName, exe, mgr.Config{
			DisplayName: windowsServiceDisplayName,
			Description: "Runs scheduled backups to PlusClouds S3 storage.",
			StartType:   mgr.StartAutomatic,
		}, args...)
		if err != nil {
			fmt.Printf("Warning: could not create the %s service: %v\n", windowsServiceName, err)
			fmt.Println("Start manually with: stash run")
			return
		}
	}
	defer service.Close() //nolint:errcheck

	// Restart after a crash; the failure count resets after a day.
	restart := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: time.Minute},
		{Type: mgr.ServiceRestart, Delay: 5 * time.Minute},
	}
	if err := service.SetRecoveryActions(restart, 86400); err != nil {
		fmt.Printf("Warning: could not set the service's restart-on-failure policy: %v\n", err)
	}

	if err := service.Start(); err != nil {
		fmt.Printf("Warning: the %s service is installed but did not start: %v\n", windowsServiceName, err)
		fmt.Println("Start it with: sc start stash")
		return
	}
	fmt.Printf("Installed and started the %s Windows service — check with: sc query stash\n", windowsServiceName)
	fmt.Println("The service runs as Local System. For SQL Server database jobs, that account needs")
	fmt.Println("permission to back up the databases — see docs/backup.agent/windows.md.")
}

// windowsCommandLine joins exe and args into the single quoted string the
// Service Control Manager stores as a service's command line.
func windowsCommandLine(exe string, args []string) string {
	line := `"` + exe + `"`
	for _, arg := range args {
		line += ` "` + arg + `"`
	}
	return line
}

// stopWindowsService asks a running service to stop and waits up to 20s.
func stopWindowsService(service *mgr.Service) {
	status, err := service.Control(svc.Stop)
	if err != nil {
		return // not running
	}
	deadline := time.Now().Add(20 * time.Second)
	for status.State != svc.Stopped && time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		if status, err = service.Query(); err != nil {
			return
		}
	}
}

// runAsSystemService runs the daemon under the Windows Service Control
// Manager when this process was started by it, reporting Running/Stopped
// and turning a Stop or Shutdown request into a context cancellation.
// Returns handled=false when started from a normal shell, so the caller
// falls back to its own signal handling.
func runAsSystemService(run func(context.Context) error) (handled bool, err error) {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return false, nil
	}
	handler := &windowsServiceHandler{run: run}
	if err := svc.Run(windowsServiceName, handler); err != nil {
		return true, err
	}
	return true, handler.err
}

type windowsServiceHandler struct {
	run func(context.Context) error
	err error
}

func (h *windowsServiceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.run(ctx) }()

	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case err := <-done:
			// The daemon ended on its own (bad config, revoked agent…).
			// A non-zero exit code makes Windows apply the restart policy.
			h.err = err
			if err != nil {
				return false, 1
			}
			return false, 0
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				status <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
					h.err = err
				}
				return false, 0
			}
		}
	}
}
