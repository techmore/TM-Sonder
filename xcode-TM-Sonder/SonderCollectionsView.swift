import SwiftUI

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
