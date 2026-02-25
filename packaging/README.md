# Packaging automation

Tagged releases (`v*`) generate Homebrew and Scoop manifests from `dist/checksums.txt` with release-accurate hashes.

## What is generated

- Homebrew formula: `dist/packaging/git-doc.rb`
- Scoop manifest: `dist/packaging/git-doc.json`

These artifacts are uploaded by the release workflow and then optionally published to external repositories.

## Required GitHub Actions configuration

Repository variables:

- `HOMEBREW_TAP_REPO` (example: `your-org/homebrew-tap`)
- `SCOOP_BUCKET_REPO` (example: `your-org/scoop-bucket`)

Repository secrets:

- `HOMEBREW_TAP_GITHUB_TOKEN` (write access to `HOMEBREW_TAP_REPO`)
- `SCOOP_BUCKET_GITHUB_TOKEN` (write access to `SCOOP_BUCKET_REPO`)

Optional path overrides in `.github/workflows/release.yml`:

- `HOMEBREW_FORMULA_PATH` (default: `Formula/git-doc.rb`)
- `SCOOP_MANIFEST_PATH` (default: `bucket/git-doc.json`)

If variables or secrets are missing, release still succeeds and publish steps are skipped.

## Local validation

Use synthetic checksums to validate the render scripts locally:

```bash
tmpdir="$(mktemp -d)"
printf '%s\n' \
	"111 git-doc_1.2.3_darwin_amd64.tar.gz" \
	"222 git-doc_1.2.3_darwin_arm64.tar.gz" \
	"333 git-doc_1.2.3_linux_amd64.tar.gz" \
	"444 git-doc_1.2.3_linux_arm64.tar.gz" \
	"555 git-doc_1.2.3_windows_amd64.zip" \
	"666 git-doc_1.2.3_windows_arm64.zip" > "$tmpdir/checksums.txt"

bash scripts/release/render_homebrew_formula.sh 1.2.3 "$tmpdir/checksums.txt" "$tmpdir/git-doc.rb" "<owner/repo>"
bash scripts/release/render_scoop_manifest.sh 1.2.3 "$tmpdir/checksums.txt" "$tmpdir/git-doc.json" "<owner/repo>"
```
