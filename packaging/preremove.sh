#!/bin/sh
# Stop and disable the unit before files are removed, so systemd is not left
# holding a reference to a binary that no longer exists.
#
# The database under /var/lib/sys-sentient is deliberately left in place: an
# uninstall is not a request to destroy a machine's monitoring history, and a
# reinstall should pick up where it left off. Removing it is the operator's
# decision to make explicitly.
set -e

if command -v systemctl >/dev/null 2>&1; then
    systemctl stop sys-sentient >/dev/null 2>&1 || true
    systemctl disable sys-sentient >/dev/null 2>&1 || true
fi
