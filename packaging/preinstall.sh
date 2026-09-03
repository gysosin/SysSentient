#!/bin/sh
# Create the service account before any file lands, so ownership is right the
# first time rather than being fixed up afterwards.
#
# The daemon runs unprivileged. It joins systemd-journal because that is the
# working log source under the shipped unit — dmesg is deliberately
# unreachable, since the unit sets ProtectKernelLogs and drops all
# capabilities.
set -e

if ! getent passwd sys-sentient >/dev/null 2>&1; then
    useradd --system \
        --home-dir /var/lib/sys-sentient \
        --shell /usr/sbin/nologin \
        --comment "SysSentient monitoring daemon" \
        sys-sentient 2>/dev/null || \
    adduser --system --no-create-home --disabled-login sys-sentient 2>/dev/null || true
fi

# Best effort: the group does not exist on non-systemd hosts, and the daemon
# degrades to its other log sources rather than failing.
if getent group systemd-journal >/dev/null 2>&1; then
    usermod -aG systemd-journal sys-sentient 2>/dev/null || true
fi
