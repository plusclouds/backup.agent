# PlusClouds Ubuntu Agent Makefile

BINARY_AGENT        := plusclouds-agent
BINARY_CTL          := plsctl
BINARY_BACKUP_AGENT := stash
BINARY_LINUX        := plusclouds.linux
BINARY_CTL_LINUX    := plsctl.linux
BINARY_WINDOWS      := plusclouds.exe
BINARY_BACKUP_LINUX   := stash.linux
BINARY_BACKUP_WINDOWS := stash.exe
BINARY_BACKUP_DARWIN  := stash.darwin
BINARY_DESKTOP        := stash-desktop
CMD_AGENT      := ./cmd/agent
CMD_CTL        := ./cmd/ctl
CMD_BACKUP_AGENT := ./cmd/backup-agent
BUILD_DIR      := ./bin

VERSION        ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT         ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# RPM's Version tag can't contain '-' (it's the Version/Release separator),
# but git describe's output usually does (e.g. "742199d-dirty" or
# "v1.2.0-5-gabcdef") — collapse hyphens to dots for RPM. rpmbuild doesn't
# reject a leading 'v' the way dpkg-deb does, but it's non-conventional for
# an RPM Version field (and can confuse rpmvercmp-based comparisons), so
# strip it here too, for the same reason as DEB_VERSION below.
RPM_VERSION    := $(patsubst v%,%,$(subst -,.,$(VERSION)))
RPM_TOPDIR     := $(CURDIR)/dist/rpmbuild

# Debian's Version field must start with a digit — git describe's output
# does whenever it's a bare commit hash (e.g. "742199d-dirty", which starts
# with '7') but not once a tag is in play (e.g. "v2.0.0-dirty", which starts
# with 'v' and fails dpkg-deb with "version number does not start with
# digit"). Strip a leading 'v' for the .deb targets only — $(VERSION)
# unchanged elsewhere (e.g. embedded via -ldflags as the app's own version
# string, where "v2.0.0-dirty" is the more readable form).
DEB_VERSION    := $(patsubst v%,%,$(VERSION))

# Development build — fast, includes debug info
LDFLAGS        := -ldflags "-X github.com/plusclouds/ubuntu-agent/internal/config.AgentVersion=$(VERSION)"

# Production build — static binary, stripped, no debug info
LDFLAGS_PROD   := -ldflags "-s -w -X github.com/plusclouds/ubuntu-agent/internal/config.AgentVersion=$(VERSION)"

# stash has its own version line (internal/backup/config.AgentVersion),
# independent of the VM agent's — see internal/backup/config/config.go.
LDFLAGS_BACKUP        := -ldflags "-X github.com/plusclouds/ubuntu-agent/internal/backup/config.AgentVersion=$(VERSION)"
LDFLAGS_BACKUP_PROD   := -ldflags "-s -w -X github.com/plusclouds/ubuntu-agent/internal/backup/config.AgentVersion=$(VERSION)"

INSTALL_DIR    := /usr/local/bin
SERVICE_DIR    := /etc/systemd/system
CONFIG_DIR     := /etc/plusclouds
LOG_DIR        := /var/log/plusclouds
STATE_DIR      := /var/lib/plusclouds

.PHONY: all build build-agent build-ctl build-stash build-prod build-linux build-ctl-linux build-windows \
	build-stash-linux build-stash-windows build-stash-darwin build-all \
	build-stash-desktop build-stash-desktop-linux build-stash-desktop-windows build-stash-desktop-darwin \
	test lint clean install install-stash uninstall uninstall-stash \
	package-deb package-deb-stash package-deb-stash-desktop \
	package-rpm-stash package-rpm-stash-desktop help

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

## build-ctl-linux: build production plsctl CLI for Linux amd64 → bin/plsctl.linux
build-ctl-linux:
	@echo "Building $(BINARY_CTL_LINUX) $(VERSION) (linux/amd64)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -trimpath $(LDFLAGS_PROD) \
		-o $(BUILD_DIR)/$(BINARY_CTL_LINUX) $(CMD_CTL)
	@echo "Done: $(BUILD_DIR)/$(BINARY_CTL_LINUX)"

## build-windows: build production binary for Windows amd64 → bin/plusclouds.exe
build-windows:
	@echo "Building $(BINARY_WINDOWS) $(VERSION) (windows/amd64)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
		go build -trimpath $(LDFLAGS_PROD) \
		-o $(BUILD_DIR)/$(BINARY_WINDOWS) $(CMD_AGENT)
	@echo "Done: $(BUILD_DIR)/$(BINARY_WINDOWS)"
	@echo "Size: $$(du -sh $(BUILD_DIR)/$(BINARY_WINDOWS) | cut -f1)"

## build-stash: build the stash daemon+CLI (development)
build-stash:
	@echo "Building $(BINARY_BACKUP_AGENT) (dev)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS_BACKUP) -o $(BUILD_DIR)/$(BINARY_BACKUP_AGENT) $(CMD_BACKUP_AGENT)
	@echo "Done: $(BUILD_DIR)/$(BINARY_BACKUP_AGENT)"

## build-stash-linux: build production stash binary for Linux amd64
build-stash-linux:
	@echo "Building $(BINARY_BACKUP_LINUX) $(VERSION) (linux/amd64)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -trimpath $(LDFLAGS_BACKUP_PROD) \
		-o $(BUILD_DIR)/$(BINARY_BACKUP_LINUX) $(CMD_BACKUP_AGENT)
	@echo "Done: $(BUILD_DIR)/$(BINARY_BACKUP_LINUX)"

## build-stash-windows: build production stash binary for Windows amd64
build-stash-windows:
	@echo "Building $(BINARY_BACKUP_WINDOWS) $(VERSION) (windows/amd64)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
		go build -trimpath $(LDFLAGS_BACKUP_PROD) \
		-o $(BUILD_DIR)/$(BINARY_BACKUP_WINDOWS) $(CMD_BACKUP_AGENT)
	@echo "Done: $(BUILD_DIR)/$(BINARY_BACKUP_WINDOWS)"

## build-stash-darwin: build production stash binary for macOS amd64
## NOTE: this repo has never produced a darwin binary before stash —
## run this early when touching stash code as a portability smoke check.
build-stash-darwin:
	@echo "Building $(BINARY_BACKUP_DARWIN) $(VERSION) (darwin/amd64)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 \
		go build -trimpath $(LDFLAGS_BACKUP_PROD) \
		-o $(BUILD_DIR)/$(BINARY_BACKUP_DARWIN) $(CMD_BACKUP_AGENT)
	@echo "Done: $(BUILD_DIR)/$(BINARY_BACKUP_DARWIN)"

## build-stash-desktop: build the desktop GUI (dev mode, current OS) — requires the
## `wails` CLI (go install github.com/wailsapp/wails/v2/cmd/wails@latest).
## WAILS_TAGS is only needed on Linux distros that ship libwebkit2gtk-4.1-dev
## instead of the older 4.0 (Wails' default) — e.g. WAILS_TAGS=webkit2_41.
WAILS_TAGS ?=
build-stash-desktop:
	@echo "Building desktop GUI (dev)..."
	cd desktop && wails build $(if $(WAILS_TAGS),-tags $(WAILS_TAGS))

## build-stash-desktop-linux/-windows/-darwin: production desktop GUI builds.
## Unlike stash's CGO_ENABLED=0 pure-Go cross-compilation, Wails links
## against each OS's native webview (WebView2/WebKitGTK/WKWebView) via cgo —
## these targets must be run FROM a machine of that OS (or an equivalent CI
## runner), not cross-compiled from Linux; each is a thin wrapper around
## `wails build -platform` for that reason.
build-stash-desktop-linux:
	cd desktop && wails build -platform linux/amd64 $(if $(WAILS_TAGS),-tags $(WAILS_TAGS))

build-stash-desktop-windows:
	cd desktop && wails build -platform windows/amd64

build-stash-desktop-darwin:
	cd desktop && wails build -platform darwin/amd64

## build-all: build production binaries for all platforms (VM agent + stash)
build-all: build-linux build-windows build-stash-linux build-stash-windows build-stash-darwin
	@echo ""
	@echo "All platform binaries:"
	@ls -lh $(BUILD_DIR)/plusclouds.* $(BUILD_DIR)/stash.*

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

## install-stash: install the stash binary and default config (requires root)
## Deployed independently of the VM agent — different machines, different
## identity model. Unlike the VM agent's `install` target, this doesn't
## install/enable the systemd unit itself — `stash register` does that
## automatically the moment it succeeds (see cmd/backup-agent/service_linux.go),
## so registering as root is now the only step needed to end up running as a
## service; this target just gets the binary and directories in place first.
install-stash: build-stash-linux
	@echo "Installing $(BINARY_BACKUP_AGENT) to $(INSTALL_DIR)..."
	install -m 0755 $(BUILD_DIR)/$(BINARY_BACKUP_LINUX) $(INSTALL_DIR)/$(BINARY_BACKUP_AGENT)
	@echo "Creating directories..."
	@mkdir -p $(CONFIG_DIR) $(LOG_DIR) $(STATE_DIR)
	@chmod 0750 $(LOG_DIR) $(STATE_DIR)
	@if [ ! -f $(CONFIG_DIR)/stash.yaml ]; then \
		echo "No config yet — run: $(INSTALL_DIR)/$(BINARY_BACKUP_AGENT) register --token=<token> --endpoint=<url>"; \
	else \
		echo "Config already exists at $(CONFIG_DIR)/stash.yaml — skipping."; \
	fi
	@echo ""
	@echo "Install complete. Next step:"
	@echo "  $(INSTALL_DIR)/$(BINARY_BACKUP_AGENT) register --token=<token> --endpoint=<url>"
	@echo "  (as root, this installs, enables, and starts stash.service automatically)"

## uninstall-stash: remove the stash binary and systemd unit (does not remove config, state, or logs)
uninstall-stash:
	systemctl disable --now stash 2>/dev/null || true
	rm -f $(INSTALL_DIR)/$(BINARY_BACKUP_AGENT)
	rm -f $(SERVICE_DIR)/stash.service
	systemctl daemon-reload
	@echo "Uninstall complete. Config, state, and logs preserved."

## package-deb: build a .deb package for distribution
## Copies the production linux builds under their installed names —
## the systemd unit execs /usr/local/bin/$(BINARY_AGENT), so the .deb
## must ship them as plusclouds-agent/plsctl, not plusclouds.linux.
package-deb: build-linux build-ctl-linux
	@echo "Packaging .deb ($(DEB_VERSION))..."
	@mkdir -p dist/deb/DEBIAN
	@mkdir -p dist/deb/usr/local/bin
	@mkdir -p dist/deb/etc/systemd/system
	@mkdir -p dist/deb/etc/plusclouds
	@mkdir -p dist/deb/var/log/plusclouds
	cp $(BUILD_DIR)/$(BINARY_LINUX)     dist/deb/usr/local/bin/$(BINARY_AGENT)
	cp $(BUILD_DIR)/$(BINARY_CTL_LINUX) dist/deb/usr/local/bin/$(BINARY_CTL)
	cp systemd/plusclouds-agent.service dist/deb/etc/systemd/system/
	cp configs/agent.yaml dist/deb/etc/plusclouds/agent.yaml
	@printf "Package: plusclouds-agent\n\
Version: $(DEB_VERSION)\n\
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
	fakeroot dpkg-deb --build dist/deb dist/plusclouds-agent_$(DEB_VERSION)_amd64.deb
	@echo "Package built: dist/plusclouds-agent_$(DEB_VERSION)_amd64.deb"

## package-deb-stash: build a .deb package for stash, independent of the VM agent's package
## No systemd unit shipped in the package itself: `stash register` installs,
## enables, and starts it automatically the moment it's run as root (see
## cmd/backup-agent/service_linux.go) — the same reasoning as install-stash.
package-deb-stash: build-stash-linux
	@echo "Packaging stash .deb ($(DEB_VERSION))..."
	@mkdir -p dist/deb-stash/DEBIAN
	@mkdir -p dist/deb-stash/usr/local/bin
	@mkdir -p dist/deb-stash/etc/plusclouds
	@mkdir -p dist/deb-stash/var/log/plusclouds
	@mkdir -p dist/deb-stash/var/lib/plusclouds
	cp $(BUILD_DIR)/$(BINARY_BACKUP_LINUX) dist/deb-stash/usr/local/bin/$(BINARY_BACKUP_AGENT)
	@printf "Package: stash\n\
Version: $(DEB_VERSION)\n\
Section: utils\n\
Priority: optional\n\
Architecture: amd64\n\
Maintainer: PlusClouds <support@plusclouds.com>\n\
Description: PlusClouds Stash backup agent\n\
 A Kopia-backed backup agent that runs on any customer machine.\n" > dist/deb-stash/DEBIAN/control
	@printf "#!/bin/sh\n\
set -e\n\
mkdir -p /var/log/plusclouds /var/lib/plusclouds\n\
chmod 0750 /var/log/plusclouds /var/lib/plusclouds\n" > dist/deb-stash/DEBIAN/postinst
	@chmod 0755 dist/deb-stash/DEBIAN/postinst
	@printf "#!/bin/sh\n\
set -e\n\
systemctl disable --now stash 2>/dev/null || true\n" > dist/deb-stash/DEBIAN/prerm
	@chmod 0755 dist/deb-stash/DEBIAN/prerm
	fakeroot dpkg-deb --build dist/deb-stash dist/stash_$(DEB_VERSION)_amd64.deb
	@echo "Package built: dist/stash_$(DEB_VERSION)_amd64.deb"

## package-deb-stash-desktop: build a .deb package for the desktop GUI (requires `wails`, see build-stash-desktop-linux)
## The desktop app is Wails+cgo, linked against the host's libwebkit2gtk at
## build time — WAILS_TAGS must match the .deb's declared Depends: default
## (Ubuntu 20.04/22.04, Debian 11/12) links libwebkit2gtk-4.0-37; pass
## WAILS_TAGS=webkit2_41 (Ubuntu 24.04+, which dropped the 4.0 package) to
## link against libwebkit2gtk-4.1-0 instead. Build on a matching distro (or
## equivalent container), same cross-compile caveat as build-stash-desktop-linux.
## Ships no systemd unit: the app manages its own per-user autostart unit at
## runtime (see desktop/autostart_linux.go), triggered from its own Settings
## toggle, not at install time.
WEBKIT_DEPENDS := $(if $(findstring webkit2_41,$(WAILS_TAGS)),libwebkit2gtk-4.1-0,libwebkit2gtk-4.0-37)
# Fedora/RHEL split the same way Debian/Ubuntu did — webkit2gtk3 provides the
# 4.0 API, webkit2gtk4.1 the split-out 4.1 one (Fedora 39+); same
# WAILS_TAGS=webkit2_41 knob picks the matching Requires:.
RPM_WEBKIT_REQUIRES := $(if $(findstring webkit2_41,$(WAILS_TAGS)),webkit2gtk4.1,webkit2gtk3)
package-deb-stash-desktop: build-stash-desktop-linux
	@echo "Packaging stash-desktop .deb ($(DEB_VERSION), webkit dep: $(WEBKIT_DEPENDS))..."
	@rm -rf dist/deb-stash-desktop
	@mkdir -p dist/deb-stash-desktop/DEBIAN
	@mkdir -p dist/deb-stash-desktop/usr/bin
	@mkdir -p dist/deb-stash-desktop/usr/share/applications
	@mkdir -p dist/deb-stash-desktop/usr/share/icons
	cp desktop/build/bin/$(BINARY_DESKTOP) dist/deb-stash-desktop/usr/bin/$(BINARY_DESKTOP)
	cp desktop/build/linux/stash-desktop.desktop dist/deb-stash-desktop/usr/share/applications/
	cp -r desktop/build/linux/hicolor dist/deb-stash-desktop/usr/share/icons/
	@printf "Package: stash-desktop\n\
Version: $(DEB_VERSION)\n\
Section: utils\n\
Priority: optional\n\
Architecture: amd64\n\
Depends: libgtk-3-0, $(WEBKIT_DEPENDS)\n\
Maintainer: PlusClouds <support@plusclouds.com>\n\
Description: PlusClouds Stash desktop app\n\
 Desktop GUI for registering and managing PlusClouds backup jobs on this machine.\n" > dist/deb-stash-desktop/DEBIAN/control
	@printf "#!/bin/sh\n\
set -e\n\
update-desktop-database /usr/share/applications 2>/dev/null || true\n\
gtk-update-icon-cache /usr/share/icons/hicolor 2>/dev/null || true\n" > dist/deb-stash-desktop/DEBIAN/postinst
	@chmod 0755 dist/deb-stash-desktop/DEBIAN/postinst
	@printf "#!/bin/sh\n\
set -e\n\
update-desktop-database /usr/share/applications 2>/dev/null || true\n\
gtk-update-icon-cache /usr/share/icons/hicolor 2>/dev/null || true\n" > dist/deb-stash-desktop/DEBIAN/postrm
	@chmod 0755 dist/deb-stash-desktop/DEBIAN/postrm
	fakeroot dpkg-deb --build dist/deb-stash-desktop dist/stash-desktop_$(DEB_VERSION)_amd64.deb
	@echo "Package built: dist/stash-desktop_$(DEB_VERSION)_amd64.deb"

## package-rpm-stash: build an .rpm package for stash (requires rpmbuild)
## No systemd unit shipped — same reasoning as package-deb-stash: `stash
## register` installs/enables/starts it automatically as root.
package-rpm-stash: build-stash-linux
	@echo "Packaging stash .rpm ($(RPM_VERSION))..."
	@mkdir -p $(RPM_TOPDIR)/SOURCES $(RPM_TOPDIR)/SPECS $(RPM_TOPDIR)/BUILD $(RPM_TOPDIR)/BUILDROOT $(RPM_TOPDIR)/RPMS $(RPM_TOPDIR)/SRPMS
	cp $(BUILD_DIR)/$(BINARY_BACKUP_LINUX) $(RPM_TOPDIR)/SOURCES/stash
	rpmbuild --define "_topdir $(RPM_TOPDIR)" --define "_rpm_version $(RPM_VERSION)" \
		-bb rpm/stash.spec
	@mkdir -p dist
	cp $(RPM_TOPDIR)/RPMS/x86_64/stash-$(RPM_VERSION)-1*.rpm dist/
	@echo "Package built: $$(ls dist/stash-$(RPM_VERSION)-1*.rpm)"

## package-rpm-stash-desktop: build an .rpm package for the desktop GUI (requires rpmbuild and `wails`, see build-stash-desktop-linux)
## Same WAILS_TAGS=webkit2_41 knob as package-deb-stash-desktop selects the
## matching Requires: — see RPM_WEBKIT_REQUIRES above.
package-rpm-stash-desktop: build-stash-desktop-linux
	@echo "Packaging stash-desktop .rpm ($(RPM_VERSION), webkit dep: $(RPM_WEBKIT_REQUIRES))..."
	@mkdir -p $(RPM_TOPDIR)/SOURCES $(RPM_TOPDIR)/SPECS $(RPM_TOPDIR)/BUILD $(RPM_TOPDIR)/BUILDROOT $(RPM_TOPDIR)/RPMS $(RPM_TOPDIR)/SRPMS
	cp desktop/build/bin/$(BINARY_DESKTOP) $(RPM_TOPDIR)/SOURCES/stash-desktop
	cp desktop/build/linux/stash-desktop.desktop $(RPM_TOPDIR)/SOURCES/
	rm -rf $(RPM_TOPDIR)/SOURCES/hicolor
	cp -r desktop/build/linux/hicolor $(RPM_TOPDIR)/SOURCES/
	rpmbuild --define "_topdir $(RPM_TOPDIR)" --define "_rpm_version $(RPM_VERSION)" \
		--define "_rpm_webkit_requires $(RPM_WEBKIT_REQUIRES)" \
		-bb rpm/stash-desktop.spec
	@mkdir -p dist
	cp $(RPM_TOPDIR)/RPMS/x86_64/stash-desktop-$(RPM_VERSION)-1*.rpm dist/
	@echo "Package built: $$(ls dist/stash-desktop-$(RPM_VERSION)-1*.rpm)"

## help: display this help
help:
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
