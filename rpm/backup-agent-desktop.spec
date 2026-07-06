Name: backup-agent-desktop
Version: %{_rpm_version}
Release: 1%{?dist}
Summary: PlusClouds Backup desktop app
License: Proprietary
URL: https://github.com/plusclouds/backup.agent
BuildArch: x86_64
Requires: gtk3
Requires: %{_rpm_webkit_requires}

# Binary-only package: Wails already produced the linked binary; nothing to
# compile here. See rpm/backup-agent.spec for why debug_package is disabled.
%define debug_package %{nil}

%description
Desktop GUI for registering and managing PlusClouds backup jobs on this
machine. Ships no systemd unit — the app manages its own per-user autostart
unit at runtime (see desktop/autostart_linux.go), triggered from its own
Settings toggle, not at install time.

%install
mkdir -p %{buildroot}/usr/bin
mkdir -p %{buildroot}/usr/share/applications
mkdir -p %{buildroot}/usr/share/icons/hicolor
install -m 0755 %{_sourcedir}/backup-agent-desktop %{buildroot}/usr/bin/backup-agent-desktop
install -m 0644 %{_sourcedir}/backup-agent-desktop.desktop %{buildroot}/usr/share/applications/backup-agent-desktop.desktop
cp -r %{_sourcedir}/hicolor/* %{buildroot}/usr/share/icons/hicolor/

%files
/usr/bin/backup-agent-desktop
/usr/share/applications/backup-agent-desktop.desktop
/usr/share/icons/hicolor/*/apps/backup-agent-desktop.png

%post
update-desktop-database /usr/share/applications >/dev/null 2>&1 || :
gtk-update-icon-cache /usr/share/icons/hicolor >/dev/null 2>&1 || :

%postun
update-desktop-database /usr/share/applications >/dev/null 2>&1 || :
gtk-update-icon-cache /usr/share/icons/hicolor >/dev/null 2>&1 || :

%changelog
* Mon Jan 01 2024 PlusClouds <support@plusclouds.com> - 0-1
- Packaged build (see git log for real history)
