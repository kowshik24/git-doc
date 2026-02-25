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

darwin_amd64="git-doc_${version}_darwin_amd64.tar.gz"
darwin_arm64="git-doc_${version}_darwin_arm64.tar.gz"
linux_amd64="git-doc_${version}_linux_amd64.tar.gz"
linux_arm64="git-doc_${version}_linux_arm64.tar.gz"

darwin_amd64_sha="$(sha_for "$darwin_amd64")"
darwin_arm64_sha="$(sha_for "$darwin_arm64")"
linux_amd64_sha="$(sha_for "$linux_amd64")"
linux_arm64_sha="$(sha_for "$linux_arm64")"

for value in "$darwin_amd64_sha" "$darwin_arm64_sha" "$linux_amd64_sha" "$linux_arm64_sha"; do
  if [[ -z "$value" ]]; then
    echo "missing checksum for one or more Homebrew artifacts" >&2
    exit 1
  fi
done

cat > "$out_file" <<EOF
class GitDoc < Formula
  desc "Automatically update docs based on Git commits"
  homepage "https://github.com/${repo}"
  version "${version}"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/${repo}/releases/download/v#{version}/git-doc_#{version}_darwin_arm64.tar.gz"
      sha256 "${darwin_arm64_sha}"
    else
      url "https://github.com/${repo}/releases/download/v#{version}/git-doc_#{version}_darwin_amd64.tar.gz"
      sha256 "${darwin_amd64_sha}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/${repo}/releases/download/v#{version}/git-doc_#{version}_linux_arm64.tar.gz"
      sha256 "${linux_arm64_sha}"
    else
      url "https://github.com/${repo}/releases/download/v#{version}/git-doc_#{version}_linux_amd64.tar.gz"
      sha256 "${linux_amd64_sha}"
    end
  end

  def install
    bin.install "git-doc"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/git-doc version")
  end
end
EOF
