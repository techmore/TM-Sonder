import SwiftUI

#Preview {
    ContentView()
        .environmentObject(SonderLibrary(store: SonderStore(rootURL: FileManager.default.temporaryDirectory.appendingPathComponent("SonderPreview"))))
}
