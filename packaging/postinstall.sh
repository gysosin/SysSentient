#!/bin/sh
# Prepare state and register the unit. Deliberately does NOT start the service:
# the first run mints a one-time setup token that the operator has to read from
# the log to create the first administrator, and a service silently started at
# install time would emit that token into the journal before anyone is watching.
set -e

install -d -o sys-sentient -g sys-sentient -m 0750 /var/lib/sys-sentient
install -d -m 0755 /etc/sys-sentient
chown root:sys-sentient /etc/sys-sentient/config.yaml 2>/dev/null || true
chmod 0640 /etc/sys-sentient/config.yaml 2>/dev/null || true

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || true
fi

cat <<'MSG'

SysSentient installed.

  1. Review /etc/sys-sentient/config.yaml
  2. sudo systemctl enable --now sys-sentient
  3. Read the one-time setup token:
       sudo journalctl -u sys-sentient | grep -i "setup token"
  4. Open http://localhost:8080 and create the first administrator.

The daemon serves plain HTTP. Put TLS in front of it before exposing it
beyond localhost -- see /usr/share/doc/sys-sentient/ and SECURITY.md.

MSG
