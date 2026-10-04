import SwiftUI
import UIKit
import SonderAPI

struct StatPill: View {
    let value: String
    let label: String

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(value)
                .font(.headline.weight(.semibold))
            Text(label)
                .font(.caption2)
                .foregroundStyle(SonderPalette.textLight)
        }
        .foregroundStyle(SonderPalette.darkText)
        .padding(.horizontal, 10)
        .padding(.vertical, 7)
        .background(SonderPalette.accentMuted, in: Capsule())
    }
}

@MainActor
final class PosterImageCache {
    static let shared = PosterImageCache()

    private let cache = NSCache<NSURL, UIImage>()

    private init() {
        cache.countLimit = 240
        cache.totalCostLimit = 80 * 1024 * 1024
    }

    func image(for url: URL) -> UIImage? {
        cache.object(forKey: url as NSURL)
    }

    func store(_ image: UIImage, for url: URL) {
        let pixels = Int(image.size.width * image.size.height * image.scale * image.scale)
        cache.setObject(image, forKey: url as NSURL, cost: pixels * 4)
    }
}

struct CachedPosterImage<Placeholder: View>: View {
    let url: URL
    var accessToken: String = ""
    var contentMode: ContentMode = .fill
    @ViewBuilder var placeholder: () -> Placeholder
    @State private var image: UIImage?
    @State private var didFail = false

    var body: some View {
        Group {
            if let image {
                Image(uiImage: image)
                    .resizable()
                    .aspectRatio(contentMode: contentMode)
            } else if didFail {
                placeholder()
            } else {
                ProgressView()
                    .task(id: "\(url.absoluteString)|\(accessToken)") {
                        await loadImage()
                    }
            }
        }
    }

    private func loadImage() async {
        if let cached = PosterImageCache.shared.image(for: url) {
            image = cached
            didFail = false
            return
        }

        do {
            if url.isFileURL {
                let data = try await Task.detached(priority: .utility) { try Data(contentsOf: url) }.value
                image = await decodeImage(from: data)
                didFail = image == nil
                return
            }
            var request = URLRequest(url: url, timeoutInterval: 8)
            // Posters dominate initial rendering. Allow URLSession's disk-backed
            // cache to satisfy repeat launches instead of redownloading every card.
            request.cachePolicy = .returnCacheDataElseLoad
            let token = accessToken.trimmingCharacters(in: .whitespacesAndNewlines)
            if token.isEmpty == false {
                request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            }
            let (data, response) = try await URLSession.shared.data(for: request)
            guard let http = response as? HTTPURLResponse, 200..<300 ~= http.statusCode,
                  let decoded = await decodeImage(from: data) else {
                didFail = true
                return
            }
            PosterImageCache.shared.store(decoded, for: url)
            image = decoded
            didFail = false
        } catch {
            didFail = true
        }
    }

    private nonisolated func decodeImage(from data: Data) async -> UIImage? {
        await Task.detached(priority: .utility) {
            UIImage(data: data)
        }.value
    }
}

struct PosterPlaceholder: View {
    let item: SonderMediaItem
    let artworkURL: URL?
    var accessToken: String = ""

    var body: some View {
        ZStack {
            RoundedRectangle(cornerRadius: 8)
                .fill(LinearGradient(colors: posterColors, startPoint: .topLeading, endPoint: .bottomTrailing))

            if let artworkURL {
                GeometryReader { geometry in
                    CachedPosterImage(url: artworkURL, accessToken: accessToken, contentMode: .fit) {
                        fallbackIcon
                    }
                    .frame(width: geometry.size.width, height: geometry.size.height)
                    .clipped()
                }
            } else {
                fallbackIcon
            }
        }
        .clipShape(RoundedRectangle(cornerRadius: 8))
        .accessibilityHidden(true)
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderPalette.border.opacity(0.65)))
    }

    private var fallbackIcon: some View {
        Image(systemName: iconName)
            .font(.title2.weight(.semibold))
            .foregroundStyle(.white.opacity(0.92))
    }

    private var posterColors: [Color] {
        switch item.kind {
        case .movie, .all:
            return [SonderPalette.ashGrey, SonderPalette.ironGrey]
        case .tvShow:
            return [SonderPalette.ashGrey.opacity(0.78), SonderPalette.ironGrey]
        case .documentary:
            return [SonderPalette.dustGrey, SonderPalette.ironGrey]
        case .audiobook:
            return [SonderPalette.ashGrey.opacity(0.58), SonderPalette.ironGrey]
        case .ebook:
            return [SonderPalette.dustGrey, SonderPalette.ironGrey]
        }
    }

    private var iconName: String {
        switch item.kind {
        case .movie:
            return "film"
        case .tvShow:
            return "tv"
        case .documentary:
            return "camera.metering.matrix"
        case .audiobook:
            return "headphones"
        case .ebook:
            return "book"
        case .all:
            return "rectangle.stack"
        }
    }
}
