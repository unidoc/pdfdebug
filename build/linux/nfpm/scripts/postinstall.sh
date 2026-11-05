#!/bin/sh
# Loads the AppArmor profile (.deb only) and refreshes the desktop and MIME
# databases so the app shows in menus and opens PDFs.

profile=/etc/apparmor.d/unidoc-pdf-debugger
if [ -f "$profile" ] && command -v apparmor_parser >/dev/null 2>&1 && [ -d /sys/kernel/security/apparmor ]; then
  apparmor_parser -r "$profile" || echo "unidoc-pdf-debugger: could not load $profile; the app may not start on Ubuntu 23.10 or later" >&2
fi

if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database -q /usr/share/applications || true
fi
if command -v update-mime-database >/dev/null 2>&1; then
  update-mime-database /usr/share/mime || true
fi

exit 0
