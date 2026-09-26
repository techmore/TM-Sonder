# The CI toolchain gap on macOS

The Mac CI job builds on a different toolchain than the one this project is
developed on, and that gap caused a build that had been red for a long time
without anybody being able to reproduce it locally.

## What the gap is

| | version |
|---|---|
| development machine | Xcode 27.0 |
| newest on `macos-15` | 26.3.0 |
| newest on `macos-26` | 26.6.0 |

No GitHub-hosted image provides Xcode 27.0. The job is pinned to **26.6.0**
because pinning is the point: before, it ran `xcode-select -s
/Applications/Xcode.app` with an `Xcode-beta` fallback, so the compiler changed
under the project without anything in the repository changing.

## Why the same source built in one place and not the other

The failures split into two groups, and neither was reproducible locally.

**AppKit main-actor annotations.** On 26.x, `NSApp`, `NSImage` and `NSWindow` are
annotated `@MainActor`, and referencing them from a synchronous nonisolated
context is an error. On 27.0 the same code compiles. `init(minLength:)` and
friends behaved the same way. It is a difference in the SDK's annotations, not a
setting: `SWIFT_APPROACHABLE_CONCURRENCY` was tried both ways on 27.0 and
changed nothing.

**A type-checker timeout.** `SonderLibraryDerivedData.swift` built TV show groups
as `Dictionary(grouping:).map { Dictionary(grouping:).map { ... }.sorted
}.sorted`. On 26.6 that is "unable to type-check this expression in reasonable
time"; 27.0 manages it. Splitting the expression into named steps with explicit
types fixed it, because nothing then has to be inferred through the nesting.

Both were real. Neither was visible from the machine where the work was done,
which is the actual lesson: **a build that only runs on CI is a build nobody can
iterate on.** Anything that must be fixed against a toolchain you do not have is
guesswork with a two-minute feedback loop attached.

## What this means for future work

CI is the only place the 26.6 result is verified. That is workable for small,
mechanical fixes, because CI is a real verifier and the loop is short. It is not
workable for anything behavioural: a concurrency fix in the Mac app cannot be
exercised against an SDK that is not installed, so the risk is shipping a change
that compiles and is wrong.

Two ways out, neither of which has been taken:

1. **Get Xcode 27.0 onto a runner.** Either a self-hosted Mac runner (the project
   already wants a self-hosted runner on the server for deploys; one on a Mac
   would cover this), or waiting for the image.
2. **Develop on 26.6.** Lower the bar deliberately rather than by accident, so
   the gap closes from the other side and the annotations get fixed while they
   are still cheap.

Option 2 is the better default: the code then builds on both, and nobody has to
remember that CI is stricter than their own machine.

## Verifying locally

```bash
# what CI does
xcodebuild build -project xcode-TM-Sonder.xcodeproj -scheme xcode-TM-Sonder \
  -configuration Release -destination 'platform=macOS' \
  SWIFT_VERSION=6 CODE_SIGNING_ALLOWED=NO
```

That reproduces the Swift 6 language mode but **not** the 26.6 SDK. Do not read
a green local build as evidence that CI will agree.
