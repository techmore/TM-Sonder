import AVFoundation
import Foundation

/// Probes a media file's real duration, dimensions, codec, and bitrate using
/// `AVURLAsset`. Runs off the main actor; results are merged into `SonderMediaItem`
/// at scan/import time so the UI and progress bar reflect reality instead of the old
/// hardcoded 5400-second placeholder.
///
/// App Store-safe: `AVURLAsset` reads user-selected/security-scoped files without
/// requiring extra entitlements, and `load(.duration)` etc. are the async metadata
/// APIs available on macOS 12+.
struct SonderMediaProbe: Sendable {
    struct Result: Sendable {
        var durationSeconds: Double
        var width: Int?
        var height: Int?
        var codec: String?
        var bitrate: Int?
    }

    /// Sentinel returned when a file cannot be opened or has no video track.
    nonisolated static let unknown = Result(durationSeconds: 0, width: nil, height: nil, codec: nil, bitrate: nil)

    /// Probes the asset at `url`. Caller must already hold the security-scoped
    /// resource (via `startAccessingSecurityScopedResource`) if one is required.
    nonisolated static func probe(url: URL) async -> Result {
        let asset = AVURLAsset(url: url)
        return await probe(asset: asset)
    }

    nonisolated static func probe(asset: AVURLAsset) async -> Result {
        do {
            let duration = try await asset.load(.duration)
            let durationSeconds = duration.seconds.isFinite ? duration.seconds : 0

            // Pick the first video track; that is what determines player compatibility
            // and is all we need for a resolution label.
            let tracks = try await asset.loadTracks(withMediaType: .video)
            guard let track = tracks.first else {
                return Result(durationSeconds: durationSeconds, width: nil, height: nil, codec: nil, bitrate: nil)
            }

            let size = try await track.load(.naturalSize)
            let bitrate = Int(try await track.load(.estimatedDataRate))
            let codecName = firstCodecName(from: try await track.load(.formatDescriptions))

            // Correct for a display transform that rotates portrait footage.
            var width = Int(size.width)
            var height = Int(size.height)
            if let transform = try? await track.load(.preferredTransform),
               transform.a == 0 || transform.d == 0,
               width > height {
                swap(&width, &height)
            }

            return Result(
                durationSeconds: durationSeconds,
                width: width > 0 ? width : nil,
                height: height > 0 ? height : nil,
                codec: codecName,
                bitrate: bitrate > 0 ? bitrate : nil
            )
        } catch {
            return Self.unknown
        }
    }

    nonisolated private static func firstCodecName(from descriptions: [Any]) -> String? {
        guard let description = descriptions.first else { return nil }
        // CMMediaType four-char codes are accessible without importing CoreMedia via
        // the underlying CMFormatDescriptionRef. Fall back to the raw description.
        let mirror = String(describing: description)
        if let range = mirror.range(of: #"'(h264|hevc|av1|vp09|avc1|hvc1|prores|mpeg4|mp4v|drmi)'"#, options: .regularExpression) {
            return String(mirror[range]).trimmingCharacters(in: CharacterSet(charactersIn: "'"))
        }
        return nil
    }
}
