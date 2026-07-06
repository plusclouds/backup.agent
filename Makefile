# PlusClouds Ubuntu Agent Makefile

BINARY_AGENT        := plusclouds-agent
BINARY_CTL          := plsctl
BINARY_BACKUP_AGENT := backup-agent
BINARY_LINUX        := plusclouds.linux
BINARY_WINDOWS      := plusclouds.windows
BINARY_BACKUP_LINUX   := backup-agent.linux
BINARY_BACKUP_WINDOWS := backup-agent.windows
BINARY_BACKUP_DARWIN  := backup-agent.darwin
BINARY_DESKTOP        := backup-agent-desktop
CMD_AGENT      := ./cmd/agent
CMD_CTL        := ./cmd/ctl
CMD_BACKUP_AGENT := ./cmd/backup-agent
BUILD_DIR      := ./bin

VERSION        ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT         ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# RPM's Version tag can't contain '-' (it's the Version/Release separator),
# but git describe's output usually does (e.g. "742199d-dirty" or
# "v1.2.0-5-gabcdef") — collapse hyphens to dots for RPM only; the .deb
# targets use $(VERSION) unchanged since Debian allows it.
RPM_VERSION    := $(subst -,.,$(VERSION))
RPM_TOPDIR     := $(CURDIR)/dist/rpmbuild

# Development build — fast, includes debug info
LDFLAGS        := -ldflags "-X github.com/plusclouds/ubuntu-agent/internal/config.AgentVersion=$(VERSION)"

# Production build — static binary, stripped, no debug info
LDFLAGS_PROD   := -ldflags "-s -w -X github.com/plusclouds/ubuntu-agent/internal/config.AgentVersion=$(VERSION)"

# backup-agent has its own version line (internal/backup/config.AgentVersion),
# independent of the VM agent's — see internal/backup/config/config.go.
LDFLAGS_BACKUP        := -ldflags "-X github.com/plusclouds/ubuntu-agent/internal/backup/config.AgentVersion=$(VERSION)"
LDFLAGS_BACKUP_PROD   := -ldflags "-s -w -X github.com/plusclouds/ubuntu-agent/internal/backup/config.AgentVersion=$(VERSION)"

INSTALL_DIR    := /usr/local/bin
SERVICE_DIR    := /etc/systemd/system
CONFIG_DIR     := /etc/plusclouds
LOG_DIR        := /var/log/plusclouds
STATE_DIR      := /var/lib/plusclouds

.PHONY: all build build-agent build-ctl build-backup-agent build-prod build-linux build-windows \
	build-backup-agent-linux build-backup-agent-windows build-backup-agent-darwin build-all \
	build-desktop build-desktop-linux build-desktop-windows build-desktop-darwin \
	test lint clean install install-backup-agent uninstall uninstall-backup-agent \
	package-deb package-deb-backup-agent package-deb-desktop \
	package-rpm-backup-agent package-rpm-desktop help

## all: build both binaries (development)
all: build

## build: build both binaries for development (current OS/arch)
build: build-agent build-ctl

## build-agent: build the agent daemon (development)
build-agent:
	@echo "Building $(BINARY_AGENT) (dev)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_AGENT) $(CMD_AGENT)
	@echo "Done: $(BUILD_DIR)/$(BINARY_AGENT)"

## build-ctl: build the plsctl CLI (development)
build-ctl:
	@echo "Building $(BINARY_CTL) (dev)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_CTL) $(CMD_CTL)
	@echo "Done: $(BUILD_DIR)/$(BINARY_CTL)"

## build-prod: alias for build-linux (backwards compatibility)
build-prod: build-linux

## build-linux: build production binary for Linux amd64 → bin/plusclouds.linux
build-linux:
	@echo "Building $(BINARY_LINUX) $(VERSION) (linux/amd64)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -trimpath $(LDFLAGS_PROD) \
		-o $(BUILD_DIR)/$(BINARY_LINUX) $(CMD_AGENT)
	@echo "Done: $(BUILD_DIR)/$(BINARY_LINUX)"
	@echo "Size: $$(du -sh $(BUILD_DIR)/$(BINARY_LINUX) | cut -f1)"
	@file $(BUILD_DIR)/$(BINARY_LINUX)

## build-windows: build production binary for Windows amd64 → bin/plusclouds.windows
build-windows:
	@echo "Building $(BINARY_WINDOWS) $(VERSION) (windows/amd64)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
		go build -trimpath $(LDFLAGS_PROD) \
		-o $(BUILD_DIR)/$(BINARY_WINDOWS) $(CMD_AGENT)
	@echo "Done: $(BUILD_DIR)/$(BINARY_WINDOWS)"
	@echo "Size: $$(du -sh $(BUILD_DIR)/$(BINARY_WINDOWS) | cut -f1)"

## build-backup-agent: build the backup-agent daemon+CLI (development)
build-backup-agent:
	@echo "Building $(BINARY_BACKUP_AGENT) (dev)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS_BACKUP) -o $(BUILD_DIR)/$(BINARY_BACKUP_AGENT) $(CMD_BACKUP_AGENT)
	@echo "Done: $(BUILD_DIR)/$(BINARY_BACKUP_AGENT)"

## build-backup-agent-linux: build production backup-agent binary for Linux amd64
build-backup-agent-linux:
	@echo "Building $(BINARY_BACKUP_LINUX) $(VERSION) (linux/amd64)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -trimpath $(LDFLAGS_BACKUP_PROD) \
		-o $(BUILD_DIR)/$(BINARY_BACKUP_LINUX) $(CMD_BACKUP_AGENT)
	@echo "Done: $(BUILD_DIR)/$(BINARY_BACKUP_LINUX)"

## build-backup-agent-windows: build production backup-agent binary for Windows amd64
build-backup-agent-windows:
	@echo "Building $(BINARY_BACKUP_WINDOWS) $(VERSION) (windows/amd64)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
		go build -trimpath $(LDFLAGS_BACKUP_PROD) \
		-o $(BUILD_DIR)/$(BINARY_BACKUP_WINDOWS) $(CMD_BACKUP_AGENT)
	@echo "Done: $(BUILD_DIR)/$(BINARY_BACKUP_WINDOWS)"

## build-backup-agent-darwin: build production backup-agent binary for macOS amd64
## NOTE: this repo has never produced a darwin binary before backup-agent —
## run this early when touching backup-agent code as a portability smoke check.
build-backup-agent-darwin:
	@echo "Building $(BINARY_BACKUP_DARWIN) $(VERSION) (darwin/amd64)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 \
		go build -trimpath $(LDFLAGS_BACKUP_PROD) \
		-o $(BUILD_DIR)/$(BINARY_BACKUP_DARWIN) $(CMD_BACKUP_AGENT)
	@echo "Done: $(BUILD_DIR)/$(BINARY_BACKUP_DARWIN)"

## build-desktop: build the desktop GUI (dev mode, current OS) — requires the
## `wails` CLI (go install github.com/wailsapp/wails/v2/cmd/wails@latest).
## WAILS_TAGS is only needed on Linux distros that ship libwebkit2gtk-4.1-dev
## instead of the older 4.0 (Wails' default) — e.g. WAILS_TAGS=webkit2_41.
WAILS_TAGS ?=
build-desktop:
	@echo "Building desktop GUI (dev)..."
	cd desktop && wails build $(if $(WAILS_TAGS),-tags $(WAILS_TAGS))

## build-desktop-linux/-windows/-darwin: production desktop GUI builds.
## Unlike backup-agent's CGO_ENABLED=0 pure-Go cross-compilation, Wails links
## against each OS's native webview (WebView2/WebKitGTK/WKWebView) via cgo —
## these targets must be run FROM a machine of that OS (or an equivalent CI
## runner), not cross-compiled from Linux; each is a thin wrapper around
## `wails build -platform` for that reason.
build-desktop-linux:
	cd desktop && wails build -platform linux/amd64 $(if $(WAILS_TAGS),-tags $(WAILS_TAGS))

build-desktop-windows:
	cd desktop && wails build -platform windows/amd64

build-desktop-darwin:
	cd desktop && wails build -platform darwin/amd64

## build-all: build production binaries for all platforms (VM agent + backup-agent)
build-all: build-linux build-windows build-backup-agent-linux build-backup-agent-windows build-backup-agent-darwin
	@echo ""
	@echo "All platform binaries:"
	@ls -lh $(BUILD_DIR)/plusclouds.* $(BUILD_DIR)/backup-agent.*

## test: run all tests with race detector and coverage
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: run golangci-lint
lint:
	@echo "Running linter..."
	golangci-lint run ./...

## clean: remove build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR) dist
	@rm -f coverage.out coverage.html
	@echo "Clean done."

## install: install binaries, systemd unit, and default config (requires root)
install: build-prod
	@echo "Installing $(BINARY_AGENT) to $(INSTALL_DIR)..."
	install -m 0755 $(BUILD_DIR)/$(BINARY_AGENT) $(INSTALL_DIR)/$(BINARY_AGENT)
	@echo "Installing $(BINARY_CTL) to $(INSTALL_DIR)..."
	install -m 0755 $(BUILD_DIR)/$(BINARY_CTL) $(INSTALL_DIR)/$(BINARY_CTL)
	@echo "Installing systemd unit..."
	install -m 0644 systemd/plusclouds-agent.service $(SERVICE_DIR)/plusclouds-agent.service
	@echo "Creating directories..."
	@mkdir -p $(CONFIG_DIR) $(LOG_DIR)
	@chmod 0750 $(LOG_DIR)
	@if [ ! -f $(CONFIG_DIR)/agent.yaml ]; then \
		install -m 0640 configs/agent.yaml $(CONFIG_DIR)/agent.yaml; \
		echo "Installed default config to $(CONFIG_DIR)/agent.yaml"; \
	else \
		echo "Config already exists at $(CONFIG_DIR)/agent.yaml — skipping."; \
	fi
	systemctl daemon-reload
	@echo ""
	@echo "Install complete. Next steps:"
	@echo "  1. Edit $(CONFIG_DIR)/agent.yaml (set nats.agent_uuid and nats.api_key)"
	@echo "  2. systemctl enable --now plusclouds-agent"
	@echo "  3. journalctl -fu plusclouds-agent"

## uninstall: remove binaries and systemd unit (does not remove config or logs)
uninstall:
	systemctl disable --now plusclouds-agent 2>/dev/null || true
	rm -f $(INSTALL_DIR)/$(BINARY_AGENT) $(INSTALL_DIR)/$(BINARY_CTL)
	rm -f $(SERVICE_DIR)/plusclouds-agent.service
	systemctl daemon-reload
	@echo "Uninstall complete. Config and logs preserved."

## install-backup-agent: install the backup-agent binary, systemd unit, and default config (requires root)
## Deployed independently of the VM agent — different machines, different identity model.
install-backup-agent: build-backup-agent-linux
	@echo "Installing $(BINARY_BACKUP_AGENT) to $(INSTALL_DIR)..."
	install -m 0755 $(BUILD_DIR)/$(BINARY_BACKUP_LINUX) $(INSTALL_DIR)/$(BINARY_BACKUP_AGENT)
	@echo "Installing systemd unit..."
	install -m 0644 systemd/backup-agent.service $(SERVICE_DIR)/backup-agent.service
	@echo "Creating directories..."
	@mkdir -p $(CONFIG_DIR) $(LOG_DIR) $(STATE_DIR)
	@chmod 0750 $(LOG_DIR) $(STATE_DIR)
	@if [ ! -f $(CONFIG_DIR)/backup-agent.yaml ]; then \
		echo "No config yet — run: $(INSTALL_DIR)/$(BINARY_BACKUP_AGENT) register --token=<token> --endpoint=<url>"; \
	else \
		echo "Config already exists at $(CONFIG_DIR)/backup-agent.yaml — skipping."; \
	fi
	systemctl daemon-reload
	@echo ""
	@echo "Install complete. Next steps:"
	@echo "  1. $(INSTALL_DIR)/$(BINARY_BACKUP_AGENT) register --token=<token> --endpoint=<url>"
	@echo "  2. systemctl enable --now backup-agent"
	@echo "  3. journalctl -fu backup-agent"

## uninstall-backup-agent: remove the backup-agent binary and systemd unit (does not remove config, state, or logs)
uninstall-backup-agent:
	systemctl disable --now backup-agent 2>/dev/null || true
	rm -f $(INSTALL_DIR)/$(BINARY_BACKUP_AGENT)
	rm -f $(SERVICE_DIR)/backup-agent.service
	systemctl daemon-reload
	@echo "Uninstall complete. Config, state, and logs preserved."

## package-deb: build a .deb package for distribution
package-deb: build-prod
	@echo "Packaging .deb ($(VERSION))..."
	@mkdir -p dist/deb/DEBIAN
	@mkdir -p dist/deb/usr/local/bin
	@mkdir -p dist/deb/etc/systemd/system
	@mkdir -p dist/deb/etc/plusclouds
	@mkdir -p dist/deb/var/log/plusclouds
	cp $(BUILD_DIR)/$(BINARY_AGENT) dist/deb/usr/local/bin/
	cp $(BUILD_DIR)/$(BINARY_CTL)   dist/deb/usr/local/bin/
	cp systemd/plusclouds-agent.service dist/deb/etc/systemd/system/
	cp configs/agent.yaml dist/deb/etc/plusclouds/agent.yaml
	@printf "Package: plusclouds-agent\n\
Version: $(VERSION)\n\
Section: utils\n\
Priority: optional\n\
Architecture: amd64\n\
Maintainer: PlusClouds <support@plusclouds.com>\n\
Description: PlusClouds Ubuntu VM Agent\n\
 A daemon for managing PlusClouds VMs via NATS.\n" > dist/deb/DEBIAN/control
	@printf "#!/bin/sh\n\
set -e\n\
mkdir -p /var/log/plusclouds\n\
chmod 0750 /var/log/plusclouds\n\
systemctl daemon-reload\n\
systemctl enable plusclouds-agent\n" > dist/deb/DEBIAN/postinst
	@chmod 0755 dist/deb/DEBIAN/postinst
	@printf "#!/bin/sh\n\
set -e\n\
systemctl disable --now plusclouds-agent 2>/dev/null || true\n" > dist/deb/DEBIAN/prerm
	@chmod 0755 dist/deb/DEBIAN/prerm
	fakeroot dpkg-deb --build dist/deb dist/plusclouds-agent_$(VERSION)_amd64.deb
	@echo "Package built: dist/plusclouds-agent_$(VERSION)_amd64.deb"

## package-deb-backup-agent: build a .deb package for backup-agent, independent of the VM agent's package
package-deb-backup-agent: build-backup-agent-linux
	@echo "Packaging backup-agent .deb ($(VERSION))..."
	@mkdir -p dist/deb-backup-agent/DEBIAN
	@mkdir -p dist/deb-backup-agent/usr/local/bin
	@mkdir -p dist/deb-backup-agent/etc/systemd/system
	@mkdir -p dist/deb-backup-agent/etc/plusclouds
	@mkdir -p dist/deb-backup-agent/var/log/plusclouds
	@mkdir -p dist/deb-backup-agent/var/lib/plusclouds
	cp $(BUILD_DIR)/$(BINARY_BACKUP_LINUX) dist/deb-backup-agent/usr/local/bin/$(BINARY_BACKUP_AGENT)
	cp systemd/backup-agent.service dist/deb-backup-agent/etc/systemd/system/
	@printf "Package: backup-agent\n\
Version: $(VERSION)\n\
Section: utils\n\
Priority: optional\n\
Architecture: amd64\n\
Maintainer: PlusClouds <support@plusclouds.com>\n\
Description: PlusClouds backup agent\n\
 A Kopia-backed backup agent that runs on any customer machine.\n" > dist/deb-backup-agent/DEBIAN/control
	@printf "#!/bin/sh\n\
set -e\n\
mkdir -p /var/log/plusclouds /var/lib/plusclouds\n\
chmod 0750 /var/log/plusclouds /var/lib/plusclouds\n\
systemctl daemon-reload\n" > dist/deb-backup-agent/DEBIAN/postinst
	@chmod 0755 dist/deb-backup-agent/DEBIAN/postinst
	@printf "#!/bin/sh\n\
set -e\n\
systemctl disable --now backup-agent 2>/dev/null || true\n" > dist/deb-backup-agent/DEBIAN/prerm
	@chmod 0755 dist/deb-backup-agent/DEBIAN/prerm
	fakeroot dpkg-deb --build dist/deb-backup-agent dist/backup-agent_$(VERSION)_amd64.deb
	@echo "Package built: dist/backup-agent_$(VERSION)_amd64.deb"

## package-deb-desktop: build a .deb package for the desktop GUI (requires `wails`, see build-desktop-linux)
## The desktop app is Wails+cgo, linked against the host's libwebkit2gtk at
## build time — WAILS_TAGS must match the .deb's declared Depends: default
## (Ubuntu 20.04/22.04, Debian 11/12) links libwebkit2gtk-4.0-37; pass
## WAILS_TAGS=webkit2_41 (Ubuntu 24.04+, which dropped the 4.0 package) to
## link against libwebkit2gtk-4.1-0 instead. Build on a matching distro (or
## equivalent container), same cross-compile caveat as build-desktop-linux.
## Ships no systemd unit: the app manages its own per-user autostart unit at
## runtime (see desktop/autostart_linux.go), triggered from its own Settings
## toggle, not at install time.
WEBKIT_DEPENDS := $(if $(findstring webkit2_41,$(WAILS_TAGS)),libwebkit2gtk-4.1-0,libwebkit2gtk-4.0-37)
# Fedora/RHEL split the same way Debian/Ubuntu did — webkit2gtk3 provides the
# 4.0 API, webkit2gtk4.1 the split-out 4.1 one (Fedora 39+); same
# WAILS_TAGS=webkit2_41 knob picks the matching Requires:.
RPM_WEBKIT_REQUIRES := $(if $(findstring webkit2_41,$(WAILS_TAGS)),webkit2gtk4.1,webkit2gtk3)
package-deb-desktop: build-desktop-linux
	@echo "Packaging desktop .deb ($(VERSION), webkit dep: $(WEBKIT_DEPENDS))..."
	@rm -rf dist/deb-desktop
	@mkdir -p dist/deb-desktop/DEBIAN
	@mkdir -p dist/deb-desktop/usr/bin
	@mkdir -p dist/deb-desktop/usr/share/applications
	@mkdir -p dist/deb-desktop/usr/share/icons
	cp desktop/build/bin/desktop dist/deb-desktop/usr/bin/$(BINARY_DESKTOP)
	cp desktop/build/linux/backup-agent-desktop.desktop dist/deb-desktop/usr/share/applications/
	cp -r desktop/build/linux/hicolor dist/deb-desktop/usr/share/icons/
	@printf "Package: backup-agent-desktop\n\
Version: $(VERSION)\n\
Section: utils\n\
Priority: optional\n\
Architecture: amd64\n\
Depends: libgtk-3-0, $(WEBKIT_DEPENDS)\n\
Maintainer: PlusClouds <support@plusclouds.com>\n\
Description: PlusClouds Backup desktop app\n\
 Desktop GUI for registering and managing PlusClouds backup jobs on this machine.\n" > dist/deb-desktop/DEBIAN/control
	@printf "#!/bin/sh\n\
set -e\n\
update-desktop-database /usr/share/applications 2>/dev/null || true\n\
gtk-update-icon-cache /usr/share/icons/hicolor 2>/dev/null || true\n" > dist/deb-desktop/DEBIAN/postinst
	@chmod 0755 dist/deb-desktop/DEBIAN/postinst
	@printf "#!/bin/sh\n\
set -e\n\
update-desktop-database /usr/share/applications 2>/dev/null || true\n\
gtk-update-icon-cache /usr/share/icons/hicolor 2>/dev/null || true\n" > dist/deb-desktop/DEBIAN/postrm
	@chmod 0755 dist/deb-desktop/DEBIAN/postrm
	fakeroot dpkg-deb --build dist/deb-desktop dist/backup-agent-desktop_$(VERSION)_amd64.deb
	@echo "Package built: dist/backup-agent-desktop_$(VERSION)_amd64.deb"

## package-rpm-backup-agent: build an .rpm package for backup-agent (requires rpmbuild)
package-rpm-backup-agent: build-backup-agent-linux
	@echo "Packaging backup-agent .rpm ($(RPM_VERSION))..."
	@mkdir -p $(RPM_TOPDIR)/SOURCES $(RPM_TOPDIR)/SPECS $(RPM_TOPDIR)/BUILD $(RPM_TOPDIR)/BUILDROOT $(RPM_TOPDIR)/RPMS $(RPM_TOPDIR)/SRPMS
	cp $(BUILD_DIR)/$(BINARY_BACKUP_LINUX) $(RPM_TOPDIR)/SOURCES/backup-agent
	cp systemd/backup-agent.service $(RPM_TOPDIR)/SOURCES/
	rpmbuild --define "_topdir $(RPM_TOPDIR)" --define "_rpm_version $(RPM_VERSION)" \
		-bb rpm/backup-agent.spec
	@mkdir -p dist
	cp $(RPM_TOPDIR)/RPMS/x86_64/backup-agent-$(RPM_VERSION)-1*.rpm dist/
	@echo "Package built: $$(ls dist/backup-agent-$(RPM_VERSION)-1*.rpm)"

## package-rpm-desktop: build an .rpm package for the desktop GUI (requires rpmbuild and `wails`, see build-desktop-linux)
## Same WAILS_TAGS=webkit2_41 knob as package-deb-desktop selects the matching
## Requires: — see RPM_WEBKIT_REQUIRES above.
package-rpm-desktop: build-desktop-linux
	@echo "Packaging desktop .rpm ($(RPM_VERSION), webkit dep: $(RPM_WEBKIT_REQUIRES))..."
	@mkdir -p $(RPM_TOPDIR)/SOURCES $(RPM_TOPDIR)/SPECS $(RPM_TOPDIR)/BUILD $(RPM_TOPDIR)/BUILDROOT $(RPM_TOPDIR)/RPMS $(RPM_TOPDIR)/SRPMS
	cp desktop/build/bin/desktop $(RPM_TOPDIR)/SOURCES/backup-agent-desktop
	cp desktop/build/linux/backup-agent-desktop.desktop $(RPM_TOPDIR)/SOURCES/
	rm -rf $(RPM_TOPDIR)/SOURCES/hicolor
	cp -r desktop/build/linux/hicolor $(RPM_TOPDIR)/SOURCES/
	rpmbuild --define "_topdir $(RPM_TOPDIR)" --define "_rpm_version $(RPM_VERSION)" \
		--define "_rpm_webkit_requires $(RPM_WEBKIT_REQUIRES)" \
		-bb rpm/backup-agent-desktop.spec
	@mkdir -p dist
	cp $(RPM_TOPDIR)/RPMS/x86_64/backup-agent-desktop-$(RPM_VERSION)-1*.rpm dist/
	@echo "Package built: $$(ls dist/backup-agent-desktop-$(RPM_VERSION)-1*.rpm)"

## help: display this help
help:
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
