import PDFKit
import SwiftUI
import SonderAPI

/// First-class reader for PDFs served by Sonder. The request uses the same
/// authorization path as artwork and playback, so paired LAN servers work too.
struct SonderPDFReader: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var model: SonderClientModel
    let item: SonderMediaItem

    @State private var document: PDFDocument?
    @State private var errorMessage: String?
    @State private var isLoading = true

    var body: some View {
        NavigationStack {
            Group {
                if let document {
                    PDFDocumentView(document: document, initialPage: UserDefaults.standard.integer(forKey: pageKey)) { page in
                        UserDefaults.standard.set(page, forKey: pageKey)
                    }
                } else if isLoading {
                    ProgressView("Opening \(item.title)")
                } else {
                    ContentUnavailableView(
                        "Couldn’t Open PDF",
                        systemImage: "doc.badge.exclamationmark",
                        description: Text(errorMessage ?? "Sonder could not download this document.")
                    )
                }
            }
            .background(SonderPalette.background)
            .navigationTitle(item.title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                }
            }
            .task { await loadDocument() }
        }
    }

    private var pageKey: String { "sonder.pdfPage.\(model.serverBaseURL.absoluteString).\(item.id)" }

    private func loadDocument() async {
        guard document == nil else { return }
        isLoading = true
        defer { isLoading = false }

        do {
            if let localURL = model.localMediaURL(for: item),
               let localDocument = PDFDocument(url: localURL) {
                document = localDocument
                model.logEvent("PDF opened offline", detail: item.title)
                return
            }
            let request = model.authorizedMediaRequest(for: model.streamURL(for: item), timeout: 30)
            let (data, response) = try await URLSession.shared.data(for: request)
            guard let http = response as? HTTPURLResponse, 200..<300 ~= http.statusCode else {
                throw HTTPStatusError(statusCode: (response as? HTTPURLResponse)?.statusCode ?? -1)
            }
            guard let loadedDocument = PDFDocument(data: data) else {
                throw PDFReaderError.invalidDocument
            }
            document = loadedDocument
            model.logEvent("PDF opened", detail: item.title)
        } catch {
            errorMessage = error.localizedDescription
            model.logEvent("PDF open failed", detail: "\(item.title): \(error.localizedDescription)")
        }
    }
}

private enum PDFReaderError: LocalizedError {
    case invalidDocument

    var errorDescription: String? {
        "The server response was not a readable PDF."
    }
}

private struct PDFDocumentView: UIViewRepresentable {
    let document: PDFDocument
    let initialPage: Int
    let onPageChange: (Int) -> Void

    func makeCoordinator() -> Coordinator { Coordinator(onPageChange: onPageChange) }

    func makeUIView(context: Context) -> PDFView {
        let view = PDFView()
        view.autoScales = true
        view.displayMode = .singlePageContinuous
        view.displayDirection = .vertical
        view.document = document
        if let page = document.page(at: min(max(initialPage, 0), max(document.pageCount - 1, 0))) {
            view.go(to: page)
        }
        context.coordinator.observe(view)
        return view
    }

    func updateUIView(_ uiView: PDFView, context: Context) {
        if uiView.document !== document {
            uiView.document = document
        }
    }

    static func dismantleUIView(_ uiView: PDFView, coordinator: Coordinator) {
        coordinator.stop()
    }

    final class Coordinator {
        let onPageChange: (Int) -> Void
        private var observer: NSObjectProtocol?
        init(onPageChange: @escaping (Int) -> Void) { self.onPageChange = onPageChange }
        func observe(_ view: PDFView) {
            observer = NotificationCenter.default.addObserver(forName: .PDFViewPageChanged, object: view, queue: .main) { [weak view, weak self] _ in
                guard let view, let page = view.currentPage, let document = view.document else { return }
                self?.onPageChange(document.index(for: page))
            }
        }
        func stop() {
            if let observer { NotificationCenter.default.removeObserver(observer) }
            observer = nil
        }
    }
}
