import SwiftUI
import SonderAPI

/// A reading/listening home that uses catalog verification rather than treating
/// books as generic media. It intentionally keeps incomplete records visible so
/// people can fix a title instead of losing it behind a bad guessed cover.
struct ReadingLibraryTab: View {
    let kind: SonderMediaKind
    @ObservedObject var model: SonderClientModel
    @State private var selectedItem: SonderMediaItem?

    private var title: String { kind == .audiobook ? "Audio" : "Books" }
    private var subtitle: String { kind == .audiobook ? "Your listening library" : "Your reading library" }
    private var items: [SonderMediaItem] {
        let source = model.filteredItems(for: kind)
        guard kind == .audiobook else { return source }
        // Multi-file audiobooks are usually named "Title - Part 01" or
        // "Title - Chapter 01". Keep one representative on the shelf so a
        // book is never rendered as a row of duplicate covers.
        return Dictionary(grouping: source, by: Self.audiobookKey)
            .values
            .compactMap { parts in parts.sorted(by: Self.audiobookPartSort).first }
            .sorted { $0.title.localizedStandardCompare($1.title) == .orderedAscending }
    }
    private var continuing: [SonderMediaItem] { items.filter { progress in
        let value = model.progress(for: progress).percent
        return value > 0 && value < 0.98
    }}
    private var ready: [SonderMediaItem] { items.filter { $0.bookValidation?.hasPrefix("verified:") == true && $0.posterURL != nil } }
    private var needsAttention: [SonderMediaItem] { items.filter { item in
        item.bookValidation?.hasPrefix("invalid:") == true || item.bookValidation == nil || item.posterURL == nil
    }}

    var body: some View {
        NavigationStack {
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 24) {
                    SectionHeader(title: title, subtitle: subtitle, count: items.count)
                    LibrarySearchBar(model: model)

                    if continuing.isEmpty == false {
                        MediaShelf(title: kind == .audiobook ? "Continue Listening" : "Continue Reading", subtitle: "Pick up where you left off", items: Array(continuing.prefix(12)), model: model, selectedItem: $selectedItem)
                    }
                    if ready.isEmpty == false {
                        MediaShelf(title: kind == .audiobook ? "Ready to Listen" : "Ready to Read", subtitle: "Verified files with cover art", items: Array(ready.prefix(18)), model: model, selectedItem: $selectedItem)
                    }
                    if needsAttention.isEmpty == false {
                        VerificationShelf(items: needsAttention, model: model, selectedItem: $selectedItem)
                    }
                    if items.isEmpty {
                        EmptyLibraryState()
                    } else {
                        Text("All \(title)")
                            .font(.title3.bold())
                            .foregroundStyle(SonderPalette.text)
                        // Keep the card width and row height deterministic. An adaptive
                        // maximum wider than the card left long titles free to grow past
                        // their visual column on compact phones.
                        LazyVGrid(columns: [GridItem(.adaptive(minimum: MediaPosterCard.width, maximum: MediaPosterCard.width), spacing: 16)], spacing: 18) {
                            ForEach(items) { item in
                                Button { selectedItem = item } label: {
                                    MediaPosterCard(item: item, progress: model.progress(for: item), artworkURL: model.artworkURL(for: item), accessToken: model.accessToken)
                                }
                                .buttonStyle(.plain)
                            }
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
                .padding(20)
            }
            .background(SonderPalette.background.ignoresSafeArea())
            .refreshable { await model.reload() }
            .navigationTitle(title)
            .sheet(item: $selectedItem) { item in
                NavigationStack { MediaDetailView(model: model, item: item) }
            }
        }
    }
}

private extension ReadingLibraryTab {
    nonisolated static func audiobookKey(_ item: SonderMediaItem) -> String {
        let stripped = item.title
            .replacingOccurrences(of: #"(?i)[\s._-]+(?:part|chapter|track|disc|cd)\s*\d+(?:\s+of\s+\d+)?$"#, with: "", options: .regularExpression)
            .trimmingCharacters(in: .whitespacesAndNewlines)
        let folderHint = item.subtitle.replacingOccurrences(of: "Audiobook", with: "", options: .caseInsensitive)
        return "\(stripped.isEmpty ? item.title : stripped)|\(folderHint)".lowercased()
    }

    nonisolated static func audiobookPartSort(_ lhs: SonderMediaItem, _ rhs: SonderMediaItem) -> Bool {
        let left = partNumber(in: lhs.title) ?? 0
        let right = partNumber(in: rhs.title) ?? 0
        if left != right { return left < right }
        return lhs.title.localizedStandardCompare(rhs.title) == .orderedAscending
    }

    nonisolated static func partNumber(in title: String) -> Int? {
        let range = NSRange(title.startIndex..<title.endIndex, in: title)
        guard let regex = try? NSRegularExpression(pattern: #"(?i)(?:part|chapter|track|disc|cd)\s*(\d+)"#),
              let match = regex.firstMatch(in: title, range: range),
              let valueRange = Range(match.range(at: 1), in: title) else { return nil }
        return Int(title[valueRange])
    }
}

private struct VerificationShelf: View {
    let items: [SonderMediaItem]
    @ObservedObject var model: SonderClientModel
    @Binding var selectedItem: SonderMediaItem?

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Needs Attention")
                .font(.title3.bold())
                .foregroundStyle(SonderPalette.text)
            Text("These files need validation or a reliable cover. Select one to see the exact file check and cover source.")
                .font(.caption)
                .foregroundStyle(SonderPalette.textLight)
            ForEach(items.prefix(8)) { item in
                Button { selectedItem = item } label: {
                    HStack(spacing: 12) {
                        PosterPlaceholder(item: item, artworkURL: model.artworkURL(for: item), accessToken: model.accessToken)
                            .frame(width: 42, height: 62)
                        VStack(alignment: .leading, spacing: 3) {
                            Text(item.title).font(.headline).foregroundStyle(SonderPalette.text).lineLimit(1)
                            Text(item.bookValidation ?? "Not verified yet")
                                .font(.caption).foregroundStyle(SonderPalette.textLight).lineLimit(2)
                        }
                        Spacer()
                        Image(systemName: "chevron.right").foregroundStyle(SonderPalette.textLight)
                    }
                }
                .buttonStyle(.plain)
                Divider()
            }
        }
        .sonderPanel()
    }
}
