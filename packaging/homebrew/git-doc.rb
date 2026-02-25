class GitDoc < Formula
  desc "Automatically update docs based on Git commits"
  homepage "https://github.com/kowshik24/git-doc"
  version "0.1.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/kowshik24/git-doc/releases/download/v#{version}/git-doc_#{version}_darwin_arm64.tar.gz"
      sha256 "REPLACE_WITH_DARWIN_ARM64_SHA256"
    else
      url "https://github.com/kowshik24/git-doc/releases/download/v#{version}/git-doc_#{version}_darwin_amd64.tar.gz"
      sha256 "REPLACE_WITH_DARWIN_AMD64_SHA256"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/kowshik24/git-doc/releases/download/v#{version}/git-doc_#{version}_linux_arm64.tar.gz"
      sha256 "REPLACE_WITH_LINUX_ARM64_SHA256"
    else
      url "https://github.com/kowshik24/git-doc/releases/download/v#{version}/git-doc_#{version}_linux_amd64.tar.gz"
      sha256 "REPLACE_WITH_LINUX_AMD64_SHA256"
    end
  end

  def install
    bin.install "git-doc"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/git-doc version")
  end
end
