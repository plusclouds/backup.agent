Name: stash
Version: %{_rpm_version}
Release: 1%{?dist}
Summary: PlusClouds Stash backup agent
License: Proprietary
URL: https://github.com/plusclouds/backup.agent
BuildArch: x86_64

# Binary-only package: the Go binary is already built by the Makefile (same
# pre-built artifact the .deb package uses — see package-deb-stash) and
# staged into %{_sourcedir} before rpmbuild runs, so there's nothing to
# compile here.
%define debug_package %{nil}

%description
A Kopia-backed backup agent that runs on any customer machine. No systemd
unit is shipped in this package — `stash register`, run as root, installs,
enables, and starts it automatically the moment registration succeeds (see
cmd/backup-agent/service_linux.go).

%install
mkdir -p %{buildroot}/usr/local/bin
mkdir -p %{buildroot}/etc/plusclouds
mkdir -p %{buildroot}/var/log/plusclouds
mkdir -p %{buildroot}/var/lib/plusclouds
install -m 0755 %{_sourcedir}/stash %{buildroot}/usr/local/bin/stash

%files
/usr/local/bin/stash
%dir /etc/plusclouds
%dir %attr(0750,root,root) /var/log/plusclouds
%dir %attr(0750,root,root) /var/lib/plusclouds

%preun
# $1 == 0 means final removal (not an upgrade) — stop the service if
# `stash register` installed it earlier; harmless no-op otherwise.
if [ "$1" = "0" ]; then
    systemctl disable --now stash >/dev/null 2>&1 || :
fi

%changelog
* Mon Jan 01 2024 PlusClouds <support@plusclouds.com> - 0-1
- Packaged build (see git log for real history)
