// スワイプの向きを判定する純粋なロジック。
// 画面（SwiftUI）からは指が動いた量 dx, dy だけを受け取り、向きを返す。
// iPhone の部品を使わないので、PC の Swift でそのままテストできる。

public enum SwipeDirection: String, Equatable, Sendable {
    case up
    case down
    case left
    case right
    case none

    /// 画面とログに出す日本語の名前
    public var label: String {
        switch self {
        case .up: return "上スワイプ"
        case .down: return "下スワイプ"
        case .left: return "左スワイプ"
        case .right: return "右スワイプ"
        case .none: return "スワイプなし"
        }
    }
}

public struct SwipeClassifier: Sendable {
    /// これより短い動きはスワイプとみなさない（ポイント単位）
    public var minDistance: Double

    public init(minDistance: Double = 60) {
        self.minDistance = minDistance
    }

    /// 指が動いた量から向きを決める。
    /// dx: 右がプラス、dy: 下がプラス（iPhone の画面の座標と同じ向き）
    public func classify(dx: Double, dy: Double) -> SwipeDirection {
        let ax = abs(dx)
        let ay = abs(dy)
        if max(ax, ay) < minDistance { return .none }
        if ay >= ax {
            return dy > 0 ? .down : .up
        } else {
            return dx > 0 ? .right : .left
        }
    }
}
