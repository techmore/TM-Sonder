# Homebrew distribution

`VERSION` is the release source of truth. Release tags use the `vX.Y.Z`
format; the tag workflow builds macOS and Linux arm64 archives and publishes
checksums to the GitHub release.

The formula in `Formula/tm-sonder.rb` is intentionally kept in this
repository so it can be tested as a tap before moving it to a dedicated
`homebrew-tm-sonder` repository. A local tap test is:

```bash
brew tap --custom-remote local/tm-sonder \
  file:///absolute/path/to/TM-Sonder
brew install --build-from-source local/tm-sonder/tm-sonder
sonder -version
```

The formula depends on Homebrew's `ffmpeg` package, which supplies both
`ffmpeg` and `ffprobe`, and tests both executables during `brew test`. It
installs the `sonder` server binary and provides a `brew services` definition.
It does not copy media or overwrite an existing
catalog; configure `~/Library/Application Support/TM-Sonder-Server/server.json`
and use the portable data export/import flow when migrating an instance.

For a bare-metal Ubuntu/Debian install, run the shared preflight before
building or enabling the systemd service:

```bash
./deploy/prepare-install.sh --install
make linux-amd64
```

The preflight installs the `ffmpeg` package; Ubuntu's package includes
`ffprobe`.
