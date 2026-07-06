Name: backup-agent
Version: %{_rpm_version}
Release: 1%{?dist}
Summary: PlusClouds backup agent
License: Proprietary
URL: https://github.com/plusclouds/backup.agent
BuildArch: x86_64
Requires: systemd

# Binary-only package: the Go binary and systemd unit are already built by
# the Makefile (same pre-built artifacts the .deb packages — see
# package-deb-backup-agent) and staged into %{_sourcedir} before rpmbuild
# runs, so there's nothing to compile here.
%define debug_package %{nil}

%description
A Kopia-backed backup agent that runs on any customer machine.

%install
mkdir -p %{buildroot}/usr/local/bin
mkdir -p %{buildroot}/etc/systemd/system
mkdir -p %{buildroot}/etc/plusclouds
mkdir -p %{buildroot}/var/log/plusclouds
mkdir -p %{buildroot}/var/lib/plusclouds
install -m 0755 %{_sourcedir}/backup-agent %{buildroot}/usr/local/bin/backup-agent
install -m 0644 %{_sourcedir}/backup-agent.service %{buildroot}/etc/systemd/system/backup-agent.service

%files
/usr/local/bin/backup-agent
/etc/systemd/system/backup-agent.service
%dir /etc/plusclouds
%dir %attr(0750,root,root) /var/log/plusclouds
%dir %attr(0750,root,root) /var/lib/plusclouds

%post
systemctl daemon-reload >/dev/null 2>&1 || :

%preun
# $1 == 0 means final removal (not an upgrade) — same "stop, don't touch
# config/state/logs" contract as the .deb's prerm.
if [ "$1" = "0" ]; then
    systemctl disable --now backup-agent >/dev/null 2>&1 || :
fi

%postun
systemctl daemon-reload >/dev/null 2>&1 || :

%changelog
* Mon Jan 01 2024 PlusClouds <support@plusclouds.com> - 0-1
- Packaged build (see git log for real history)
