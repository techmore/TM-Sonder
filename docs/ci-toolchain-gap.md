# Pinned Apple build toolchain

The macOS and native iOS CI jobs now use GitHub's `xcode-27` hosted image and
explicitly select `/Applications/Xcode_27.0.app/Contents/Developer`. Both report
and check the selected version before building. The Mac app retains Swift 6
language mode; the native app retains its project language settings.

GitHub made this image available and moved its base OS to macOS 27 on September
16, 2026: [runner image announcement](https://github.com/actions/runner-images/issues/14404).
The image is a preview and may queue when capacity is limited.

This closes the previous mismatch between local Xcode 27 and hosted Xcode 26.6.
A future toolchain upgrade should change the runner/path together and pass both
build jobs. Native CI also tests offline persistence from a fresh checkout.
The native source is tracked directly in TM-Sonder, with dependency lockfiles;
it no longer requires an unregistered embedded Git checkout.

Unsigned CI builds establish compilation, not device playback correctness.
Physical interruption, lock-screen controls, suspension, download recovery,
and airplane-mode checks remain necessary before a device release.
