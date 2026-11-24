import SwiftUI
import OSLog
import DevKitCore

/// 試作1：スワイプすると、その向きが画面に1行ずつ出る。
/// 同じ内容を iPhone のシステムログにも出す（PC の go-ios で Claude が読むため）。
struct ContentView: View {
    @State private var lines: [String] = []
    private let classifier = SwipeClassifier(minDistance: 60)
    private let logger = Logger(subsystem: "io.github.iwataka793.devkit", category: "swipe")

    var body: some View {
        VStack(spacing: 0) {
            Text("ここをスワイプ")
                .font(.headline)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .background(Color.gray.opacity(0.15))
                .contentShape(Rectangle())
                .accessibilityIdentifier("swipeArea")
                .gesture(
                    DragGesture(minimumDistance: 10)
                        .onEnded { value in
                            let direction = classifier.classify(
                                dx: Double(value.translation.width),
                                dy: Double(value.translation.height)
                            )
                            record(direction)
                        }
                )

            List(Array(lines.enumerated().reversed()), id: \.offset) { item in
                Text(item.element)
                    .accessibilityIdentifier("line-\(item.offset)")
            }
            .frame(height: 220)
            .accessibilityIdentifier("logList")

            Text("build: \(BuildInfo.commit)")
                .font(.caption2)
                .foregroundStyle(.secondary)
                .padding(.vertical, 6)
                .accessibilityIdentifier("buildInfo")
        }
        // 画面下の端からのスワイプは、1回目をアプリが受け取る（2回目でホームに戻る）
        .defersSystemGestures(on: .bottom)
        .onAppear {
            // のぞき窓（Probe.swift）から来た命令を画面に反映する
            Probe.shared.onCommand = { what, text in
                switch what {
                case "clear":
                    lines.removeAll()
                    Probe.shared.clear()
                case "add":
                    add("PC: \(text)")
                default:
                    break
                }
            }
            Probe.shared.start()
        }
    }

    private func record(_ direction: SwipeDirection) {
        let time = Date().formatted(date: .omitted, time: .standard)
        add("\(direction.label)  \(time)")
        logger.notice("DEVKIT swipe=\(direction.rawValue, privacy: .public) commit=\(BuildInfo.commit, privacy: .public)")
    }

    private func add(_ line: String) {
        lines.append(line)
        Probe.shared.record(line)
    }
}
