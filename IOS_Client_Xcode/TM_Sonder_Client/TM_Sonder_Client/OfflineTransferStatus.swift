import SwiftUI

/// A concrete transfer state avoids treating an active download as a disabled
/// action. It deliberately reports both completion and live throughput.
struct OfflineTransferStatus: View {
    let progress: SonderOfflineDownloadProgress?
    let isPaused: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 7) {
            if let fraction = progress?.fractionCompleted {
                ProgressView(value: fraction)
                    .tint(SonderPalette.ironGrey)
                Text(progress?.isMediaDuration == true
                     ? "\(Int((fraction * 100).rounded()))% of movie saved"
                     : "\(Int((fraction * 100).rounded()))% • \(ByteCountFormatter.string(fromByteCount: progress?.bytesWritten ?? 0, countStyle: .file)) of \(ByteCountFormatter.string(fromByteCount: progress?.bytesExpected ?? 0, countStyle: .file))")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(SonderPalette.textLight)
            } else {
                if !isPaused { ProgressView().controlSize(.small) }
                Text(isPaused ? "Ready to resume" : "Preparing download…")
                    .font(.caption)
                    .foregroundStyle(SonderPalette.textLight)
            }

            Text(isPaused ? "Paused" : speedText)
                .font(.caption.weight(.semibold).monospacedDigit())
                .foregroundStyle(isPaused ? SonderPalette.textLight : SonderPalette.ironGrey)
        }
        .accessibilityElement(children: .combine)
    }

    private var speedText: String {
        let bytesPerSecond = progress?.bytesPerSecond ?? 0
        guard bytesPerSecond > 0 else { return "Downloading…" }
        let rate = "\(ByteCountFormatter.string(fromByteCount: Int64(bytesPerSecond), countStyle: .file))/s"
        guard let progress, progress.bytesExpected > progress.bytesWritten else { return rate }
        let remaining = Double(progress.bytesExpected - progress.bytesWritten) / bytesPerSecond
        guard remaining.isFinite, remaining < 86_400 else { return rate }
        return "\(rate) · About \(SonderTime.format(remaining)) left"
    }
}
