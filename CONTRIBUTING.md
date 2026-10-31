# Contributing to StageBox

Read [README.md](README.md) for setup and [CODEX.md](CODEX.md) for architecture,
data flow, build commands and platform traps. StageBox targets Windows x64;
Linux stubs are useful for development but do not validate Windows behavior.

Use Go as specified in `go.mod`, Node.js 22.12+ and npm, Wails CLI v2.11.0,
Python 3.11+, .NET SDK 10+, and MinGW-w64 for Linux-to-Windows builds.
Install frontend dependencies with `npm --prefix frontend ci`, copy
`config.example.json` to `config.json`, and edit your local games path.
Keep local configuration, logs and controller profiles out of commits.

Keep changes focused and explain the visible behavior, relevant platform and
validation in the pull request. Update README for user-facing changes and
CODEX for architecture, data flow, build steps or traps introduced by the change.
Preserve Windows build tags and non-Windows stubs. Keep generated Wails bindings
in sync with bound Go methods. Avoid unrelated vendor changes.

Useful build commands:

```sh
npm --prefix frontend run build
make virtual-helper-deps
make build-win
make release
```

The maintainer runs tests. When proposing a change, list the relevant commands
and manual Windows checks for the maintainer to run; do not claim checks that
were not performed. For controller and launch changes, the usual starting point
on Windows is:

```sh
go test ./internal/platform/controller ./internal/games
```

Use the issue templates for reproducible bugs and feature proposals. Include
StageBox version, Windows version, controller model/connection where relevant,
expected behavior and reproduction steps. Redact local paths, account names,
network names and device identifiers before attaching log excerpts. Report
security-sensitive issues using [SECURITY.md](SECURITY.md).

StageBox's original code is licensed under GPL-3.0-only; contributions are
provided under the same license. Third-party components retain their own
licenses as described in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
