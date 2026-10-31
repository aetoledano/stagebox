# Security

StageBox is pre-1.0 software for Windows kiosk and living-room setups. Security
fixes target the current development version; older snapshots may need to be
upgraded.

For a vulnerability, use **Security → Report a vulnerability** on the GitHub
repository when private reporting is enabled. If that option is unavailable,
ask the maintainer for a private reporting channel without posting exploit
details, credentials, or unredacted logs in a public issue. Include the affected
version, reproduction steps, impact and any proposed fix in the private report.

The kiosk installer can replace the Windows shell and optionally configure
automatic logon. Windows AutoAdminLogon stores the account password in the
Winlogon registry; the installer asks before enabling it. Review those options
before using the installer on a shared machine.

StageBox normally runs unelevated. Virtual input uses the bundled elevated
helper and drivers. Keep `stagebox.exe` with its matching `assets/hidmaestro`
directory: the executable verifies its helper manifest before elevation.
Preserve bundled license notices and use the documented driver installers.

Logs can contain local paths, SSIDs and controller identifiers. Review and
redact excerpts before sharing them. Do not include `config.json`, controller
profiles, mapping backups or the HidHide recovery journal in public reports.
