# Publishing StageBox

Repository: [aetoledano/stagebox](https://github.com/aetoledano/stagebox).
The configured `origin` uses `https://github.com/aetoledano/stagebox.git`; its
public default branch is `main`. The local development branch is `develop`.
Git HTTPS authentication uses the repository-local `gh auth git-credential`
helper and the maintainer's existing `gh auth login` session. SSH keys are not
required for this setup. Publish explicitly with `git push origin develop:main`
because the local and remote branch names differ.
No GitHub Actions workflow is included in the initial publication. The owner
deferred automation; a workflow to build Windows releases will be added later.
When adding it, the GitHub CLI OAuth login needs `workflow` scope; grant that
interactively with `gh auth refresh -h github.com -s workflow` when needed.

GitHub's license-only initial commit was merged into the project history before
publication, preserving both histories without replacing the remote branch.

The public repository preserves the existing Git history. Earlier commits
include author identity/email and local development paths; ignoring a file
affects future commits only. Local `config.json` and generated Wails dependency
fingerprints are now untracked and remain on the development machine.
Unused artwork/font files were removed from the current tree, but remain in
earlier commits. The active artwork inventory is in `docs/ARTWORK.md`.

Before the first public push:

- Confirm the artwork provenance described in THIRD_PARTY_NOTICES.md.
- Review the staged diff and new files. Keep local configuration, logs,
  controller profiles, downloaded helpers and generated release bundles out
  of Git. The intentionally embedded SDL DLL and vendored runtime files stay.
- Have the maintainer run `go test ./internal/platform/controller ./internal/games`
  on Windows and check native, Steam and RetroBat sessions, controller return,
  remapping, virtual input/rumble and dialog navigation on the target hardware.
- Build the paired Windows application/helper with `make release`. Confirm
  the resulting directory contains the executable, its entire helper folder,
  setup scripts, example config, GPL license and dependency notices.
- Create an empty GitHub repository, then configure its remote and push the
  intended branch. Do not initialize a second README/license on GitHub.
- Enable private vulnerability reporting in GitHub's security settings if you
  want reporters to use the private route described in SECURITY.md.

Builds, dependency audits and tests are currently manual. Tests remain
maintainer-run. The future release workflow should build the matching Windows
executable/helper bundle and include the license and dependency notices.

For each binary release, publish the complete `make release` bundle with a
matching source tag. Rebuild the Go executable whenever the helper manifest
changes. Include corresponding source for the distributed build, including
any modified vendored dependencies; do not publish a binary paired with an
unrelated source snapshot. A source-only public repository does not require
publishing a Windows binary at the same time.
