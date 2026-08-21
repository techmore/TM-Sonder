import AppKit
import SwiftUI

enum SonderTheme {
    private static var palette = SonderThemePalette.earthy

    static func apply(preset: SonderThemePreset) {
        palette = preset.palette
    }

    static var background: Color { palette.background }
    static var sidebar: Color { palette.sidebar }
    static var surface: Color { palette.surface }
    static var surfaceDeep: Color { palette.surfaceDeep }
    static var border: Color { palette.border }
    static var accent: Color { palette.accent }
    static var accentStrong: Color { palette.accentStrong }
    static var accentMuted: Color { palette.accentMuted }
    static var text: Color { palette.text }
    static var textLight: Color { palette.textLight }
    static var darkText: Color { palette.darkText }
}

enum SonderThemePreset: String, Codable, CaseIterable, Identifiable, Sendable {
    case earthy

    var id: String { rawValue }

    nonisolated var label: String {
        switch self {
        case .earthy: "Earthy Tones"
        }
    }

    nonisolated var description: String {
        switch self {
        case .earthy: "Gentle sage leads the palette, with creamy white, sandy taupe, warm mauve-pink accents, and organic charcoal."
        }
    }

    var palette: SonderThemePalette {
        switch self {
        case .earthy:
            return SonderThemePalette(
                background: Color(hex: "#B0C4B1").opacity(0.22),
                sidebar: Color(hex: "#B0C4B1").opacity(0.34),
                surface: Color(hex: "#F7E1D7").opacity(0.92),
                surfaceDeep: Color(hex: "#DEDBD2"),
                border: Color(hex: "#B0C4B1").opacity(0.88),
                accent: Color(hex: "#4A5759"),
                accentStrong: Color(hex: "#4A5759"),
                accentMuted: Color(hex: "#DEDBD2"),
                text: Color(hex: "#4A5759"),
                textLight: Color(hex: "#4A5759").opacity(0.74),
                darkText: Color(hex: "#F7E1D7")
            )
        }
    }
}

struct SonderThemePalette {
    var background: Color
    var sidebar: Color
    var surface: Color
    var surfaceDeep: Color
    var border: Color
    var accent: Color
    var accentStrong: Color
    var accentMuted: Color
    var text: Color
    var textLight: Color
    var darkText: Color

    static let earthy = SonderThemePalette(
        background: Color(hex: "#B0C4B1").opacity(0.22),
        sidebar: Color(hex: "#B0C4B1").opacity(0.34),
        surface: Color(hex: "#F7E1D7").opacity(0.92),
        surfaceDeep: Color(hex: "#DEDBD2"),
        border: Color(hex: "#B0C4B1").opacity(0.88),
        accent: Color(hex: "#4A5759"),
        accentStrong: Color(hex: "#4A5759"),
        accentMuted: Color(hex: "#DEDBD2"),
        text: Color(hex: "#4A5759"),
        textLight: Color(hex: "#4A5759").opacity(0.74),
        darkText: Color(hex: "#F7E1D7")
    )
}

enum SonderTime {
    static func format(_ seconds: Double) -> String {
        let safeSeconds = max(0, Int(seconds))
        let hours = safeSeconds / 3600
        let minutes = (safeSeconds % 3600) / 60
        if hours > 0 {
            return "\(hours)h \(minutes)m"
        }
        return "\(minutes)m"
    }
}

extension JSONEncoder {
    nonisolated static var sonder: JSONEncoder {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        encoder.dateEncodingStrategy = .iso8601
        return encoder
    }
}

extension JSONDecoder {
    nonisolated static var sonder: JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return decoder
    }
}

extension Color {
    init(hex: String) {
        let cleaned = hex.trimmingCharacters(in: CharacterSet.alphanumerics.inverted)
        var value: UInt64 = 0
        Scanner(string: cleaned).scanHexInt64(&value)
        let r, g, b, a: Double
        switch cleaned.count {
        case 6:
            r = Double((value >> 16) & 0xFF) / 255
            g = Double((value >> 8) & 0xFF) / 255
            b = Double(value & 0xFF) / 255
            a = 1
        case 8:
            r = Double((value >> 24) & 0xFF) / 255
            g = Double((value >> 16) & 0xFF) / 255
            b = Double((value >> 8) & 0xFF) / 255
            a = Double(value & 0xFF) / 255
        default:
            r = 0
            g = 0
            b = 0
            a = 1
        }
        self.init(.sRGB, red: r, green: g, blue: b, opacity: a)
    }

    var hexString: String {
        #if canImport(AppKit)
        let nsColor = NSColor(self).usingColorSpace(.sRGB) ?? NSColor.black
        #else
        let nsColor = UIColor(self).usingColorSpace(.sRGB) ?? UIColor.black
        #endif
        let r = Int((nsColor.redComponent * 255).rounded())
        let g = Int((nsColor.greenComponent * 255).rounded())
        let b = Int((nsColor.blueComponent * 255).rounded())
        return String(format: "#%02X%02X%02X", r, g, b)
    }
}

extension String {
    nonisolated var isSonderPlaceholderSummary: Bool {
        let value = trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        return value.isEmpty || value.contains("imported") || value.contains("indexed from") || value.contains("placeholder")
    }

    nonisolated var cleanedMediaTitle: String {
        replacingOccurrences(of: ".", with: " ")
            .replacingOccurrences(of: "_", with: " ")
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    nonisolated var plexSafeName: String {
        let illegal = CharacterSet(charactersIn: "/:\\?%*|\"<>")
        return components(separatedBy: illegal)
            .joined(separator: " ")
            .replacingOccurrences(of: "\\s+", with: " ", options: .regularExpression)
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    nonisolated var removingPlexTags: String {
        replacingOccurrences(of: #"\{(?:imdb|tmdb|audible|audnexus)-[^}]+\}"#, with: "", options: [.regularExpression, .caseInsensitive])
            .replacingOccurrences(of: #"\{edition-[^}]+\}"#, with: "", options: [.regularExpression, .caseInsensitive])
    }

    nonisolated var removingSplitSuffix: String {
        replacingOccurrences(of: #"(?i)[\s._-]+(?:cd\d+|disc\d+|disk\d+|dvd\d+|part\d+|pt\d+)$"#, with: "", options: .regularExpression)
    }

    nonisolated func firstMatch(pattern: String) -> [String]? {
        guard let regex = try? NSRegularExpression(pattern: pattern, options: [.caseInsensitive]) else { return nil }
        let nsRange = NSRange(startIndex..<endIndex, in: self)
        guard let match = regex.firstMatch(in: self, range: nsRange), match.numberOfRanges > 1 else { return nil }
        return (1..<match.numberOfRanges).compactMap { index in
            guard let range = Range(match.range(at: index), in: self) else { return nil }
            return String(self[range]).cleanedMediaTitle
        }
    }
}
