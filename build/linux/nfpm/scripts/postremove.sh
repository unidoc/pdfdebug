#!/bin/sh
# Unloads the AppArmor profile when the .deb is removed (dpkg keeps the
# profile file until purge, so it is still readable here; RPM passes a number
# as $1) and refreshes the desktop database so PDFs no longer offer the app.

profile=/etc/apparmor.d/unidoc-pdf-debugger
case "$1" in
  remove)
    if [ -f "$profile" ] && command -v apparmor_parser >/dev/null 2>&1 && [ -d /sys/kernel/security/apparmor ]; then
      apparmor_parser -R "$profile" || true
    fi
    ;;
esac

if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database -q /usr/share/applications || true
fi

exit 0
