import SwiftUI
import WebKit
import SonderAPI

struct SonderEPUBReader: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var model: SonderClientModel
    let item: SonderMediaItem
    @State private var book: SonderEPUB?
    @State private var section = 0
    @State private var errorMessage: String?
    @State private var fontSize = 18.0

    private var bookmarkKey: String { "sonder.epub.\(model.serverBaseURL.absoluteString).\(item.id)" }

    var body: some View {
        NavigationStack {
            Group {
                if let book {
                    VStack(spacing: 0) {
                        EPUBSectionView(url: book.chapters[section], root: book.root, fontSize: fontSize, bookmarkKey: "\(bookmarkKey).\(section)")
                            .id("\(section)-\(fontSize)")
                        HStack {
                            Button { section -= 1 } label: { Image(systemName: "chevron.left").frame(width: 44, height: 44) }
                                .disabled(section == 0).accessibilityLabel("Previous section")
                            Spacer()
                            Menu("Section \(section + 1) of \(book.chapters.count)") {
                                ForEach(book.chapters.indices, id: \.self) { index in
                                    Button("Section \(index + 1)") { section = index }
                                }
                            }
                            Spacer()
                            Button { section += 1 } label: { Image(systemName: "chevron.right").frame(width: 44, height: 44) }
                                .disabled(section == book.chapters.count - 1).accessibilityLabel("Next section")
                        }.padding(.horizontal)
                    }
                } else if let errorMessage {
                    ContentUnavailableView("Couldn’t Open EPUB", systemImage: "book.closed", description: Text(errorMessage))
                } else {
                    ProgressView("Opening book…")
                }
            }
            .navigationTitle(item.title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Menu {
                        Button("Larger text") { fontSize = min(fontSize + 2, 32) }
                        Button("Smaller text") { fontSize = max(fontSize - 2, 14) }
                    } label: { Image(systemName: "textformat.size") }
                    .accessibilityLabel("Text size")
                }
                ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } }
            }
            .task {
                do {
                    guard let url = model.localMediaURL(for: item) else { throw EPUBError.invalidBook }
                    let loaded = try await Task.detached(priority: .userInitiated) { try SonderEPUB.open(url) }.value
                    section = min(max(UserDefaults.standard.integer(forKey: bookmarkKey), 0), loaded.chapters.count - 1)
                    book = loaded
                } catch { errorMessage = error.localizedDescription }
            }
            .onChange(of: section) { _, value in UserDefaults.standard.set(value, forKey: bookmarkKey) }
            .onDisappear { if let book { try? FileManager.default.removeItem(at: book.root) } }
        }
    }
}

private struct EPUBSectionView: UIViewRepresentable {
    let url: URL
    let root: URL
    let fontSize: Double
    let bookmarkKey: String

    func makeCoordinator() -> Coordinator { Coordinator(bookmarkKey: bookmarkKey, root: root) }

    func makeUIView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        config.websiteDataStore = .nonPersistent()
        config.defaultWebpagePreferences.allowsContentJavaScript = false
        let view = WKWebView(frame: .zero, configuration: config)
        view.navigationDelegate = context.coordinator
        view.scrollView.delegate = context.coordinator
        do {
            let data = try Data(contentsOf: url)
            guard data.count <= 8 * 1024 * 1024, let html = String(data: data, encoding: .utf8) else { throw EPUBError.invalidBook }
            // Restrict all book-authored resources to this local book. No scripts,
            // frames, forms, or remote tracking requests are allowed.
            let policy = "<meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; img-src file: data:; style-src file: 'unsafe-inline'; font-src file: data:;\">"
            let style = "<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><style>html{color-scheme:light dark}body{font-size:\(fontSize)px!important;line-height:1.6!important;padding:16px;overflow-wrap:anywhere}img,svg{max-width:100%;height:auto}</style>"
            let rendered = url.deletingLastPathComponent().appendingPathComponent(".sonder-\(UUID().uuidString).html")
            try (policy + style + html).write(to: rendered, atomically: true, encoding: .utf8)
            view.loadFileURL(rendered, allowingReadAccessTo: root)
        } catch {
            view.loadHTMLString("<p>This section could not be read.</p>", baseURL: nil)
        }
        return view
    }
    func updateUIView(_ view: WKWebView, context: Context) {}

    final class Coordinator: NSObject, WKNavigationDelegate, UIScrollViewDelegate {
        let bookmarkKey: String
        let root: URL
        private var restored = false
        init(bookmarkKey: String, root: URL) { self.bookmarkKey = bookmarkKey; self.root = root }
        func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
            let fraction = min(max(UserDefaults.standard.double(forKey: bookmarkKey), 0), 1)
            webView.evaluateJavaScript("window.scrollTo(0, (document.documentElement.scrollHeight-window.innerHeight)*\(fraction));") { [weak self] _, _ in self?.restored = true }
        }
        func scrollViewDidScroll(_ scrollView: UIScrollView) {
            guard restored else { return }
            let distance = scrollView.contentSize.height - scrollView.bounds.height
            guard distance > 0 else { return }
            UserDefaults.standard.set(min(max(scrollView.contentOffset.y / distance, 0), 1), forKey: bookmarkKey)
        }
        func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
            // Section navigation is controlled by the reader, never by active
            // content or external URLs supplied by an imported book.
            guard let url = navigationAction.request.url else { decisionHandler(.cancel); return }
            let local = url.isFileURL && url.standardizedFileURL.path.hasPrefix(root.standardizedFileURL.path + "/")
            decisionHandler(navigationAction.navigationType == .other && (local || url.absoluteString == "about:blank") ? .allow : .cancel)
        }
    }
}
