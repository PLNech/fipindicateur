# Homebrew formula for a personal tap (e.g. PLNech/homebrew-tap).
# Source build: brew compiles against the user's own mpv (which ships libmpv),
# so the cgo/libmpv link is a non-issue by construction, exactly like `make
# build`. See packaging/homebrew/README.md for how to create the tap.
class Fipindicateur < Formula
  desc "Tiny system-tray client for the FIP (Radio France) webradios"
  homepage "https://github.com/PLNech/fipindicateur"
  url "https://github.com/PLNech/fipindicateur/archive/refs/tags/v0.3.0.tar.gz"
  sha256 "85c50bf34ada1766cb783413c7c0513d52a83ee3dd49530a9c6bf61c7a86987f"
  license "GPL-3.0-or-later"
  head "https://github.com/PLNech/fipindicateur.git", branch: "main"

  depends_on "go" => :build
  depends_on "pkg-config" => :build
  depends_on "mpv"

  def install
    ldflags = "-X github.com/PLNech/fipindicateur/internal/version.Version=v#{version}"
    system "go", "build", *std_go_args(ldflags: ldflags, output: bin/"fipindicateur"), "./cmd/fipindicateur"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/fipindicateur version")
  end
end
