import Foundation
import Network
import OSLog

/// のぞき窓：PC（Claude の iPhone の道具 iphone_devkit）から USB 経由でアプリの中の様子を聞く口。
/// iPhone の中のループバックの 8765 番だけで待つ（Wi-Fi など外からは届かない）。
///   GET /state                 … 版・画面に出ている行・行の数
///   GET /cmd?do=clear          … 行を全部消す
///   GET /cmd?do=add&text=…     … 行を 1 つ足す（画面を触らずに試すとき）
final class Probe {
    static let shared = Probe()
    static let port: UInt16 = 8765

    /// /cmd を受けたとき画面に伝える（メインスレッドで呼ぶ）。引数は do と text
    var onCommand: ((String, String) -> Void)?

    private let queue = DispatchQueue(label: "io.github.iwataka793.devkit.probe")
    private let logger = Logger(subsystem: "io.github.iwataka793.devkit", category: "probe")
    private let lock = NSLock()
    private var listener: NWListener?
    private var lines: [String] = []
    private var requests = 0

    func record(_ line: String) {
        lock.lock()
        lines.append(line)
        if lines.count > 500 { lines.removeFirst(lines.count - 500) }
        lock.unlock()
    }

    func clear() {
        lock.lock()
        lines.removeAll()
        lock.unlock()
    }

    /// 待ち受けを始める（もう待っていれば何もしない）。アプリが前に戻るたびに呼ぶ。
    func start() {
        queue.async { [self] in
            guard listener == nil else { return }
            do {
                // iPhone の中のループバックだけで待つ。USB からの接続は 127.0.0.1 から来る。
                // requiredLocalEndpoint で 127.0.0.1:8765 に縛る書き方では、ready になるのに接続が届かなかった（2026-11-25）
                let params = NWParameters.tcp
                params.allowLocalEndpointReuse = true
                params.requiredInterfaceType = .loopback
                let l = try NWListener(using: params, on: NWEndpoint.Port(rawValue: Probe.port)!)
                l.newConnectionHandler = { [weak self] c in self?.handle(c) }
                l.stateUpdateHandler = { [weak self] state in
                    guard let self else { return }
                    switch state {
                    case .ready:
                        self.logger.notice("DEVKIT probe ready port=\(Probe.port, privacy: .public)")
                    case .failed(let e):
                        self.logger.error("DEVKIT probe failed \(String(describing: e), privacy: .public)")
                        self.queue.async { self.listener?.cancel(); self.listener = nil }
                    case .cancelled:
                        self.queue.async { self.listener = nil }
                    default:
                        break
                    }
                }
                l.start(queue: queue)
                listener = l
            } catch {
                logger.error("DEVKIT probe start error \(String(describing: error), privacy: .public)")
            }
        }
    }

    private func handle(_ c: NWConnection) {
        logger.notice("DEVKIT probe accept from=\(String(describing: c.endpoint), privacy: .public)")
        c.start(queue: queue)
        c.receive(minimumIncompleteLength: 1, maximumLength: 65536) { [weak self] data, _, _, _ in
            guard let self else { c.cancel(); return }
            let req = String(decoding: data ?? Data(), as: UTF8.self)
            let first = req.components(separatedBy: "\r\n").first ?? ""
            let parts = first.split(separator: " ")
            let target = parts.count > 1 ? String(parts[1]) : "/"
            let (status, body) = self.route(target)
            let bodyData = (try? JSONSerialization.data(withJSONObject: body, options: [.sortedKeys])) ?? Data("{}".utf8)
            var out = Data("HTTP/1.1 \(status)\r\nContent-Type: application/json; charset=utf-8\r\nContent-Length: \(bodyData.count)\r\nConnection: close\r\n\r\n".utf8)
            out.append(bodyData)
            c.send(content: out, completion: .contentProcessed { _ in c.cancel() })
        }
    }

    private func route(_ target: String) -> (String, [String: Any]) {
        let comps = URLComponents(string: "http://probe" + target)
        let path = comps?.path ?? "/"
        var q: [String: String] = [:]
        for item in comps?.queryItems ?? [] { q[item.name] = item.value ?? "" }
        lock.lock()
        requests += 1
        let snapshot = lines
        let n = requests
        lock.unlock()
        logger.notice("DEVKIT probe request=\(path, privacy: .public) n=\(n, privacy: .public)")
        switch path {
        case "/", "/state":
            return ("200 OK", [
                "commit": BuildInfo.commit,
                "lines": snapshot,
                "count": snapshot.count,
                "requests": n,
                "time": ISO8601DateFormatter().string(from: Date()),
            ])
        case "/cmd":
            let what = q["do"] ?? ""
            let text = q["text"] ?? ""
            guard what == "clear" || what == "add" else {
                return ("400 Bad Request", ["error": "do は clear か add"])
            }
            DispatchQueue.main.async { self.onCommand?(what, text) }
            return ("200 OK", ["ok": true, "do": what])
        default:
            return ("404 Not Found", ["error": "道は /state と /cmd だけ"])
        }
    }
}
