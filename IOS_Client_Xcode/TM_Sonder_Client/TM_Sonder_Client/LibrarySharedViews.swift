import Foundation
import SwiftUI

struct MetadataRow: View {
    let label: String
    let value: String

    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(label)
                .foregroundStyle(SonderPalette.textLight)
            Spacer(minLength: 16)
            Text(value)
                .foregroundStyle(SonderPalette.text)
                .multilineTextAlignment(.trailing)
        }
        .font(.subheadline)
    }
}

extension View {
    func sonderPanel() -> some View {
        padding(16)
            .background(SonderPalette.surface, in: RoundedRectangle(cornerRadius: 16, style: .continuous))
    }

    func sonderInput() -> some View {
        padding(12)
            .background(SonderPalette.surfaceDeep, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}

enum SonderTime {
    static func format(_ seconds: Double) -> String {
        guard seconds.isFinite, seconds > 0 else { return "0:00" }
        let totalSeconds = Int(seconds.rounded())
        let hours = totalSeconds / 3600
        let minutes = (totalSeconds % 3600) / 60
        let remainingSeconds = totalSeconds % 60
        if hours > 0 {
            return String(format: "%d:%02d:%02d", hours, minutes, remainingSeconds)
        }
        return String(format: "%d:%02d", minutes, remainingSeconds)
    }
}
