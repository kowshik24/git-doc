#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 3 ]]; then
  echo "usage: $0 <version> <checksums-file> <output-file> [github-repo]" >&2
  exit 1
fi

version="$1"
checksums_file="$2"
out_file="$3"
repo="${4:-${GITHUB_REPOSITORY:-}}"

if [[ -z "$repo" ]]; then
  echo "github repository must be provided as arg4 or GITHUB_REPOSITORY env var" >&2
  exit 1
fi

if [[ ! -f "$checksums_file" ]]; then
  echo "checksums file not found: $checksums_file" >&2
  exit 1
fi

sha_for() {
  local artifact="$1"
  awk -v artifact="$artifact" '$2 == artifact { print $1 }' "$checksums_file"
}

windows_amd64="git-doc_${version}_windows_amd64.zip"
windows_arm64="git-doc_${version}_windows_arm64.zip"

windows_amd64_sha="$(sha_for "$windows_amd64")"
windows_arm64_sha="$(sha_for "$windows_arm64")"

if [[ -z "$windows_amd64_sha" || -z "$windows_arm64_sha" ]]; then
  echo "missing checksum for one or more Scoop artifacts" >&2
  exit 1
fi

cat > "$out_file" <<EOF
{
  "version": "${version}",
  "description": "Automatically update docs based on Git commits",
  "homepage": "https://github.com/${repo}",
  "license": "MIT",
  "architecture": {
    "64bit": {
      "url": "https://github.com/${repo}/releases/download/v${version}/git-doc_${version}_windows_amd64.zip",
      "hash": "${windows_amd64_sha}"
    },
    "arm64": {
      "url": "https://github.com/${repo}/releases/download/v${version}/git-doc_${version}_windows_arm64.zip",
      "hash": "${windows_arm64_sha}"
    }
  },
  "bin": [
    "git-doc.exe"
  ],
  "checkver": {
    "github": "https://github.com/${repo}"
  },
  "autoupdate": {
    "architecture": {
      "64bit": {
        "url": "https://github.com/${repo}/releases/download/v\$version/git-doc_\$version_windows_amd64.zip"
      },
      "arm64": {
        "url": "https://github.com/${repo}/releases/download/v\$version/git-doc_\$version_windows_arm64.zip"
      }
    }
  }
}
EOF
