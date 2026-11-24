import SwiftUI

@main
struct DevKitApp: App {
    @Environment(\.scenePhase) private var phase

    var body: some Scene {
        WindowGroup {
            ContentView()
        }
        .onChange(of: phase) { _, now in
            // 裏に回ると待ち受けが止められることがあるので、前に戻るたびに始め直す
            if now == .active { Probe.shared.start() }
        }
    }
}
