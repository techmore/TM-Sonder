import SwiftUI
import UniformTypeIdentifiers

struct CollectionsView: View {
    @ObservedObject var library: SonderLibrary
    @Binding var selectedItemID: UUID?
    @State private var newListName = ""
    @State private var newListKind: SonderCollectionKind = .playlist

    var body: some View {
        List {
            Section("Create") {
                TextField("Name", text: $newListName)
                Picker("Type", selection: $newListKind) {
                    ForEach(SonderCollectionKind.allCases) { kind in
                        Text(kind.label).tag(kind)
                    }
                }
                .pickerStyle(.segmented)
                Button {
                    library.createCollection(named: newListName, kind: newListKind)
                    newListName = ""
                } label: {
                    Label("Create", systemImage: "plus.circle")
                }
                .disabled(newListName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }

            ForEach(library.collections) { collection in
                Section("\(collection.name) - \(collection.kind.label)") {
                    ForEach(collection.itemIDs.compactMap(library.item(id:))) { item in
                        MediaListRow(item: item, detail: item.kind.label) {
                            selectedItemID = item.id
                        }
                    }
                    if collection.itemIDs.isEmpty {
                        Text("Add media from a title detail screen.")
                            .foregroundStyle(SonderTheme.textLight)
                    }
                }
            }
        }
        .navigationTitle("Collections & Playlists")
        .scrollContentBackground(.hidden)
        .background(SonderTheme.background)
    }
}

struct BookListsView: View {
    @ObservedObject var library: SonderLibrary
    @Binding var selectedItemID: UUID?
    @State private var selectedListID: String?
    @State private var showingMissingExporter = false
    @State private var missingDocument = SonderPlainTextDocument()

    private var selectedList: SonderBookList? {
        SonderBookList.curated.first { $0.id == selectedListID } ?? SonderBookList.curated.first
    }

    var body: some View {
        Group {
            if let list = selectedList {
                listDetail(list)
            } else {
                listCatalog
            }
        }
        .navigationTitle("Lists")
        .background(SonderTheme.background)
        .fileExporter(isPresented: $showingMissingExporter, document: missingDocument, contentType: .plainText, defaultFilename: "missing-books.txt") { _ in }
    }

    private var listCatalog: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                PageHeader(
                    title: "Book Lists",
                    subtitle: "Browse curated best-of-all-time shelves and apply any list to your collection."
                )

                LazyVGrid(columns: [GridItem(.adaptive(minimum: 270, maximum: 390), spacing: 16)], spacing: 16) {
                    ForEach(SonderBookList.curated) { list in
                        Button {
                            selectedListID = list.id
                        } label: {
                            listCard(list)
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
            .padding(24)
        }
        .background(SonderTheme.background)
    }

    private func listCard(_ list: SonderBookList) -> some View {
        let resolved = library.resolvedBookList(list)
        return VStack(alignment: .leading, spacing: 12) {
            HStack(alignment: .top, spacing: 12) {
                Image(systemName: list.icon)
                    .font(.title2)
                    .foregroundStyle(SonderTheme.accentStrong)
                    .frame(width: 38, height: 38)
                    .background(SonderTheme.accent.opacity(0.16), in: RoundedRectangle(cornerRadius: 10))
                VStack(alignment: .leading, spacing: 4) {
                    Text(list.name)
                        .font(.headline)
                        .foregroundStyle(SonderTheme.text)
                        .multilineTextAlignment(.leading)
                    Text("\(list.entries.count) books")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(SonderTheme.textLight)
                }
                Spacer(minLength: 0)
                Image(systemName: "chevron.right")
                    .font(.caption.weight(.bold))
                    .foregroundStyle(SonderTheme.textLight)
            }

            Text(list.subtitle)
                .font(.subheadline)
                .foregroundStyle(SonderTheme.textLight)
                .lineLimit(2)
                .frame(maxWidth: .infinity, alignment: .leading)

            VStack(alignment: .leading, spacing: 4) {
                ForEach(list.entries.prefix(3)) { entry in
                    Text("\(entry.order). \(entry.title)")
                        .font(.caption)
                        .foregroundStyle(SonderTheme.text)
                        .lineLimit(1)
                }
            }

            HStack {
                Text(resolved.missing.isEmpty ? "Ready in library" : "\(resolved.missing.count) missing")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(resolved.missing.isEmpty ? SonderTheme.accentStrong : .orange)
                Spacer()
                Text("View list")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(SonderTheme.accentStrong)
            }
        }
        .padding(18)
        .frame(maxWidth: .infinity, minHeight: 205, alignment: .topLeading)
        .background(SonderTheme.surface, in: RoundedRectangle(cornerRadius: 14))
        .overlay(RoundedRectangle(cornerRadius: 14).stroke(SonderTheme.border))
        .contentShape(RoundedRectangle(cornerRadius: 14))
    }

    private func listDetail(_ list: SonderBookList) -> some View {
        let resolved = library.resolvedBookList(list)
        return ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                Button {
                    selectedListID = nil
                } label: {
                    Label("All Lists", systemImage: "chevron.left")
                }
                .buttonStyle(.plain)
                .foregroundStyle(SonderTheme.accentStrong)

                PageHeader(title: list.name, subtitle: list.subtitle)
                HStack(spacing: 12) {
                    Button { library.applyBookList(list) } label: { Label("Apply to Collection", systemImage: "folder.badge.plus") }
                        .buttonStyle(.borderedProminent)
                    Button {
                        missingDocument = SonderPlainTextDocument(text: library.missingBookListText(list))
                        showingMissingExporter = true
                    } label: { Label("Export Missing .txt", systemImage: "square.and.arrow.down") }
                    .disabled(resolved.missing.isEmpty)
                    Spacer()
                    Text("\(resolved.matched.count) of \(list.entries.count) in library")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(SonderTheme.textLight)
                }

                DashboardPanel(title: "Reading order") {
                    VStack(spacing: 0) {
                        ForEach(list.entries) { entry in
                            let item = resolved.matched.first { $0.title.caseInsensitiveCompare(entry.title) == .orderedSame }
                            HStack(spacing: 12) {
                                Text("\(entry.order)").font(.caption.monospacedDigit()).foregroundStyle(SonderTheme.textLight).frame(width: 28, alignment: .leading)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(entry.title).font(.headline)
                                    Text(entry.author).font(.caption).foregroundStyle(SonderTheme.textLight)
                                }
                                Spacer()
                                if let item {
                                    Button("Open") { selectedItemID = item.id }
                                        .buttonStyle(.bordered)
                                    Image(systemName: "checkmark.circle.fill").foregroundStyle(SonderTheme.accentStrong)
                                } else {
                                    Label("Missing", systemImage: "cart").foregroundStyle(.orange).font(.caption.weight(.semibold))
                                }
                            }
                            .padding(.vertical, 10)
                            Divider()
                        }
                    }
                }
            }
            .padding(24)
        }
    }
}
