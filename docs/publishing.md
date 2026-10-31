# Publishing StageBox

Repository: [aetoledano/stagebox](https://github.com/aetoledano/stagebox).
The configured `origin` uses `https://github.com/aetoledano/stagebox.git`; its
public default branch is `main`. The local development branch is `develop`.
Git HTTPS authentication uses the repository-local `gh auth git-credential`
helper and the maintainer's existing `gh auth login` session. SSH keys are not
required for this setup. Publish explicitly with `git push origin develop:main`
because the local and remote branch names differ.

## Create a Windows release

1. First make sure the release workflow and desired application changes are on
   `main`. Preparing them locally does not activate GitHub Actions.
2. In GitHub, open **Releases → Draft a new release**. Create a new tag such as
   `v0.43`, choose **main** as its target, and enter the release notes.
3. Choose **Publish release**. The **Windows release bundle** workflow builds
   the exact tagged commit on Ubuntu and attaches these assets after it finishes:

   - `stagebox-v0.43-windows-x64.zip`
   - `stagebox-v0.43-windows-x64.zip.sha256`
4. Download the ZIP, extract the complete folder contents, and follow
   `START-HERE.txt`. Validate the actual launcher on Windows before recommending
   a release for general use.

Publishing a pre-release also triggers the build. Saving a draft, editing
release notes, or pushing a normal commit/tag does not. Tags must point to a
commit reachable from `main`; tags from an unmerged branch fail the source
check. Use letters, digits, dots, underscores and hyphens in release tags, up to
80 characters, starting with a letter or digit. A tag must include the workflow
file itself; an older tag from before this workflow cannot run it.

The workflow builds the tag rather than the current head of `main`, sets the
app's startup version to the release tag, runs `make release`, and packages the
paired app/helper, setup scripts, config example, quick-start instructions and
licenses. `BUILD-INFO.json` records the exact source commit. GitHub's automatic
Source code archives remain available alongside the Windows bundle.

## Verify the workflow without publishing a release

Open **Actions → Windows release bundle → Run workflow**, select `main`, and
start the run. This uses the same Linux build/packaging path, with a version such
as `check-1-1`, and saves the ZIP/checksum as an Actions artifact. The release
upload job is skipped; no GitHub release or tag is created. From the CLI, use
`gh workflow run release.yml --ref main`. This checks the hosted build path;
attaching assets to a release is exercised when you publish an actual release.

## Permissions and recovery

The workflow uses the built-in `GITHUB_TOKEN`; no personal token or custom
repository secret is needed. The Linux build job has read-only repository
access. Only the separate asset-upload job has `contents: write`, and it never
checks out or runs the application source. Action versions are pinned by commit.

To eventually push the workflow from a GitHub CLI OAuth login, GitHub may require
`workflow` scope: run `gh auth refresh -h github.com -s workflow` interactively
if that push is rejected. This is separate from the workflow's automatic token.

If building fails, open the run from the release's **Actions** page, inspect the
failed step, and rerun after resolving the cause. Completed ZIP/checksum files
are retained as an Actions artifact for 14 days, including when the release
upload fails. Rerunning just the failed upload job reuses the completed build
artifact by ID. The upload rechecks the tag's source commit and checksum.

Existing assets are not automatically overwritten. If a previous upload partly
succeeded, remove only the affected ZIP/checksum pair from that release before
rerunning its upload job. Prefer a new version/tag when application changes are
needed; do not move a published tag to different source.

This publish-then-attach flow needs release immutability to be disabled, as it
currently is in this repository. If immutable releases are enabled later, assets
must instead be uploaded to a draft before publication; see
[GitHub's immutable release guidance](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases).


GitHub's license-only initial commit was merged into the project history before
publication, preserving both histories without replacing the remote branch.

The public repository preserves the existing Git history. Earlier commits
include author identity/email and local development paths; ignoring a file
affects future commits only. Local `config.json` and generated Wails dependency
fingerprints are now untracked and remain on the development machine.
Unused artwork/font files were removed from the current tree, but remain in
earlier commits. See the [active artwork inventory](ARTWORK.md).

## Before publishing source or a binary release

- Confirm the artwork provenance described in [third-party notices](../THIRD_PARTY_NOTICES.md).
- Review the staged diff and new files. Keep local configuration, logs,
  controller profiles, downloaded helpers and generated release bundles out
  of Git. The intentionally embedded SDL DLL and vendored runtime files stay.
- Have the maintainer run `go test ./internal/platform/controller ./internal/games`
  on Windows and check native, Steam and RetroBat sessions, controller return,
  remapping, virtual input/rumble and dialog navigation on the target hardware.
- For a binary release, build the paired Windows application/helper with `make release`. Confirm
  the resulting directory contains the executable, its entire helper folder,
  setup scripts, example config, GPL license and dependency notices.
- Review the intended source branch and push it with the explicit local/remote
  mapping above. Keep the source tag aligned with any published binary bundle.
- Enable private vulnerability reporting in GitHub's security settings if you
  want reporters to use the private route described in [Security](../SECURITY.md).

Release builds and asset packaging are automated by the release workflow. Tests
and Windows hardware validation remain maintainer-run; neither job runs tests.

For each binary release, publish the complete `make release` bundle with a
matching source tag. Rebuild the Go executable whenever the helper manifest
changes. Include corresponding source for the distributed build, including
any modified vendored dependencies; do not publish a binary paired with an
unrelated source snapshot. A source-only public repository does not require
publishing a Windows binary at the same time.
