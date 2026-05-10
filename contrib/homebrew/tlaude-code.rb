class TlaudeCode < Formula
  desc "A production-grade AI coding CLI with multi-agent orchestration"
  homepage "https://github.com/tetexu/tlaude-code"
  version "1.0.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/tetexu/tlaude-code/releases/download/v#{version}/tlaude-code-darwin-arm64.tar.gz"
      sha256 "" # filled by GoReleaser
    else
      url "https://github.com/tetexu/tlaude-code/releases/download/v#{version}/tlaude-code-darwin-amd64.tar.gz"
      sha256 "" # filled by GoReleaser
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/tetexu/tlaude-code/releases/download/v#{version}/tlaude-code-linux-arm64.tar.gz"
      sha256 "" # filled by GoReleaser
    else
      url "https://github.com/tetexu/tlaude-code/releases/download/v#{version}/tlaude-code-linux-amd64.tar.gz"
      sha256 "" # filled by GoReleaser
    end
  end

  def install
    bin.install "tlaude-code"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/tlaude-code --version")
  end
end
