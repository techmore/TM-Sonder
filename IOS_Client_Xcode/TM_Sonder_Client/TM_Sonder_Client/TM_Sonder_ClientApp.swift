import SwiftUI
import SonderAPI
import UIKit

final class SonderAppDelegate: NSObject, UIApplicationDelegate {
    func application(
        _ application: UIApplication,
        handleEventsForBackgroundURLSession identifier: String,
        completionHandler: @escaping () -> Void
    ) {
        guard identifier == "com.techmore.tmsonder.offline-downloads" else {
            completionHandler()
            return
        }
        SonderBackgroundDownloadCoordinator.shared.setBackgroundEventsCompletionHandler(completionHandler)
    }
}

@main
struct TM_Sonder_ClientApp: App {
    @UIApplicationDelegateAdaptor(SonderAppDelegate.self) private var appDelegate

    var body: some Scene {
        WindowGroup {
            ContentView()
        }
    }
}
