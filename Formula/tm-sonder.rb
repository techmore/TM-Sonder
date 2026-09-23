class TmSonder < Formula
  desc "Personal media and audiobook server"
  homepage "https://github.com/techmore/TM-Sonder"
  url "https://github.com/techmore/TM-Sonder/archive/refs/tags/v0.2.1.tar.gz"
  version "0.2.1"
  sha256 "73ff168ff06b051f1c57d5d222b623b9a81bbbc7f2c68a55d03e9809dc668962"

  depends_on "go" => :build
  depends_on "ffmpeg"

  def install
    ldflags = "-s -w -X main.version=#{version} -X tm-sonder/server/internal/httpapi.Version=#{version} -X tm-sonder/server/internal/httpapi.Build=homebrew"

    cd "server" do
      system "go", "build", "-trimpath", "-ldflags", ldflags, "-o", bin/"sonder", "./cmd/sonder"
    end

    mkdir_p etc/"sonder"
    cp "deploy/homebrew-server.json", etc/"sonder/server.json.example"
  end

  service do
    run [opt_bin/"sonder", "-config", etc/"sonder/server.json"]
    keep_alive true
    log_path var/"log/tm-sonder.log"
    error_log_path var/"log/tm-sonder.log"
  end

  test do
    assert_match "sonder #{version}", shell_output("#{bin}/sonder -version")
  end
end
