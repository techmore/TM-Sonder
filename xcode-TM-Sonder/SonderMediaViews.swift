import SwiftUI

struct MediaDetailView: View {
    let item: SonderMediaItem
    @ObservedObject var library: SonderLibrary
    @State private var seconds: Double
    @State private var duration: Double

    init(item: SonderMediaItem, library: SonderLibrary) {
        self.item = item
        self.library = library
        let progress = library.progressRecord(for: item)
        _seconds = State(initialValue: progress?.seconds ?? item.progressSeconds)
        _duration = State(initialValue: progress?.duration ?? item.durationSeconds)
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                HStack(alignment: .top, spacing: 20) {
                    PosterView(item: item)
                        .frame(width: 170, height: 250)
                        .clipShape(RoundedRectangle(cornerRadius: 8))

                    VStack(alignment: .leading, spacing: 10) {
                        Text(item.title)
                            .font(.largeTitle.weight(.bold))
                            .foregroundStyle(SonderTheme.text)
                        Text(item.subtitle)
                            .font(.title3)
                            .foregroundStyle(SonderTheme.textLight)
                        Text(item.summary)
                            .foregroundStyle(SonderTheme.text)
                        TagCloud(tags: item.tags)

                        HStack {
                            Button {
                                library.play(item)
                            } label: {
                                Label(item.hasFile ? "Play" : "Locate File", systemImage: item.hasFile ? "play.fill" : "folder")
                            }
                            .buttonStyle(.borderedProminent)

                            Button {
                                SonderSystemServices.shared.openLocalWebInterface(port: UInt16(clamping: library.serverSettings.port), path: "/stream/\(item.id.uuidString)")
                            } label: {
                                Label("Stream URL", systemImage: "network")
                            }
                            .disabled(item.hasFile == false)
                        }

                        if item.kind == .movie || item.kind == .tvShow || item.kind == .documentary {
                            HStack {
                                Button {
                                    library.renameForPlex(item)
                                } label: {
                                    Label("Rename for Plex", systemImage: "textformat")
                                }
                                .disabled(item.hasFile == false)

                                Button {
                                    library.convertToMP4(item)
                                } label: {
                                    Label("Convert to MP4", systemImage: "arrow.triangle.2.circlepath")
                                }
                                .disabled(item.hasFile == false)
                            }
                        }
                    }
                }

                DashboardPanel(title: "Watch Progress") {
                    VStack(alignment: .leading, spacing: 12) {
                        ProgressView(value: duration <= 0 ? 0 : seconds / duration)
                            .tint(SonderTheme.accentStrong)

                        HStack {
                            Stepper("Position: \(SonderTime.format(seconds))", value: $seconds, in: 0...max(duration, 1), step: 60)
                            Spacer()
                            Text("\(Int((duration <= 0 ? 0 : seconds / duration) * 100))%")
                                .font(.body.monospacedDigit())
                                .foregroundStyle(SonderTheme.textLight)
                        }

                        Button {
                            library.updateProgress(itemID: item.id, seconds: seconds, duration: duration)
                        } label: {
                            Label("Save Progress", systemImage: "checkmark")
                        }
                    }
                }

                DashboardPanel(title: "Metadata") {
                    MetadataRow(label: "Kind", value: item.kind.label)
                    MetadataRow(label: "Studio", value: item.studio)
                    MetadataRow(label: "Year", value: "\(item.year)")
                    MetadataRow(label: "Runtime", value: item.durationSeconds > 0 ? SonderTime.format(item.durationSeconds) : "Not probed")
                    MetadataRow(label: "Format", value: item.format.rawValue.uppercased())
                    if let width = item.probedWidth, let height = item.probedHeight {
                        MetadataRow(label: "Resolution", value: "\(width)×\(height)")
                    } else {
                        MetadataRow(label: "Resolution", value: "Not probed")
                    }
                    MetadataRow(label: "Codec", value: item.probedCodec ?? "Not probed")
                    if let bitrate = item.probedBitrate {
                        MetadataRow(label: "Bitrate", value: "\(bitrate / 1_000_000) Mbps")
                    }
                    MetadataRow(label: "Browser Play", value: item.isBrowserPlayable ? "Yes" : "Catalog only")
                    if item.kind == .tvShow {
                        MetadataRow(label: "Show", value: item.showTitle ?? item.title)
                        MetadataRow(label: "Episode", value: item.episodeCode)
                    } else if item.kind == .ebook {
                        MetadataRow(label: "Reader", value: "Books / Preview")
                    } else if item.kind == .audiobook {
                        MetadataRow(label: "Player", value: "Books / Music")
                    }
                    MetadataRow(label: "Plex Name", value: item.plexFileName)
                }

                DashboardPanel(title: "Add to Collection") {
                    FlowLayout {
                        ForEach(library.collections) { collection in
                            Button(collection.name) {
                                library.add(item, to: collection)
                            }
                            .buttonStyle(.bordered)
                        }
                    }
                }
            }
            .padding(24)
        }
        .background(SonderTheme.background)
        .navigationTitle("Title")
        .onAppear {
            library.prioritizeAssets(for: item.id)
        }
        .onChange(of: item.id) { _, _ in
            library.prioritizeAssets(for: item.id)
            let progress = library.progressRecord(for: item)
            seconds = progress?.seconds ?? item.progressSeconds
            duration = progress?.duration ?? item.durationSeconds
        }
    }
}

struct MediaCard: View {
    let item: SonderMediaItem
    let progress: Double
    let posterHeight: CGFloat
    let action: () -> Void
    let play: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Button(action: action) {
                PosterView(item: item)
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.plain)
            .frame(maxWidth: .infinity)
            .frame(height: posterHeight)
            .clipShape(RoundedRectangle(cornerRadius: 8))
            .contentShape(RoundedRectangle(cornerRadius: 8))
            .clipped()

            VStack(alignment: .leading, spacing: 4) {
                Text(item.title)
                    .font(.headline)
                    .lineLimit(2)
                Text(item.subtitle)
                    .font(.caption)
                    .foregroundStyle(SonderTheme.textLight)
                    .lineLimit(1)
                Text(item.kind.label)
                    .font(.caption2.weight(.semibold))
                    .foregroundStyle(SonderTheme.textLight)
                    .lineLimit(1)
            }
            .frame(minHeight: 56, alignment: .top)

            Text(item.summary)
                .font(.caption)
                .foregroundStyle(SonderTheme.textLight)
                .lineLimit(5)
                .fixedSize(horizontal: false, vertical: true)
                .frame(minHeight: 70, alignment: .top)

            ProgressView(value: progress)
                .tint(SonderTheme.accentStrong)

            Spacer(minLength: 0)

            Button(action: play) {
                Label(item.hasFile ? "Play" : "Locate", systemImage: item.hasFile ? "play.fill" : "folder")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
        }
        .padding(12)
        .frame(maxWidth: .infinity, minHeight: posterHeight + 272, maxHeight: posterHeight + 272, alignment: .top)
        .background(SonderTheme.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderTheme.border))
        .clipShape(RoundedRectangle(cornerRadius: 8))
    }
}

struct PosterView: View {
    let item: SonderMediaItem

    var body: some View {
        ZStack {
            if let posterImage = item.localPosterImage {
                Image(nsImage: posterImage)
                    .resizable()
                    .scaledToFill()
            } else {
                LinearGradient(colors: item.kind.gradient, startPoint: .topLeading, endPoint: .bottomTrailing)
                Image(systemName: item.kind.icon)
                    .font(.system(size: 52, weight: .semibold))
                    .foregroundStyle(SonderTheme.accent.opacity(0.55))
            }
        }
        .overlay(alignment: .topLeading) {
            Text(item.kind.shortLabel)
                .font(.caption2.weight(.heavy))
                .padding(.horizontal, 7)
                .padding(.vertical, 5)
                .foregroundStyle(SonderTheme.darkText)
                .background(SonderTheme.accent.opacity(0.92), in: RoundedRectangle(cornerRadius: 4))
                .padding(8)
        }
        .overlay(alignment: .bottomLeading) {
            VStack(alignment: .leading, spacing: 4) {
                Text(item.title)
                    .font(.caption.weight(.bold))
                    .lineLimit(3)
                Text(item.format.rawValue.uppercased())
                    .font(.system(size: 9, weight: .heavy))
            }
            .foregroundStyle(SonderTheme.darkText)
            .padding(8)
            .background(SonderTheme.accent.opacity(0.92), in: RoundedRectangle(cornerRadius: 6))
            .padding(8)
        }
    }
}

struct EmptyDetailView: View {
    let title: String
    let subtitle: String
    let icon: String

    var body: some View {
        VStack(spacing: 10) {
            Image(systemName: icon)
                .font(.system(size: 42, weight: .semibold))
                .foregroundStyle(SonderTheme.accentStrong)
            Text(title)
                .font(.title2.weight(.bold))
                .foregroundStyle(SonderTheme.text)
            Text(subtitle)
                .multilineTextAlignment(.center)
                .foregroundStyle(SonderTheme.textLight)
                .frame(maxWidth: 360)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(SonderTheme.background)
    }
}

struct MediaListRow: View {
    let item: SonderMediaItem
    let detail: String
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            HStack(spacing: 12) {
                PosterView(item: item)
                    .frame(width: 38, height: 56)
                    .clipShape(RoundedRectangle(cornerRadius: 6))
                VStack(alignment: .leading, spacing: 3) {
                    Text(item.title)
                        .foregroundStyle(SonderTheme.text)
                    Text(detail)
                        .font(.caption)
                        .foregroundStyle(SonderTheme.textLight)
                }
                Spacer()
            }
        }
    }
}
