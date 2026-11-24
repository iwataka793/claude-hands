import XCTest
@testable import DevKitCore

final class SwipeTests: XCTestCase {
    let classifier = SwipeClassifier(minDistance: 60)

    func testDown() {
        XCTAssertEqual(classifier.classify(dx: 5, dy: 120), .down)
    }

    func testUp() {
        XCTAssertEqual(classifier.classify(dx: -10, dy: -200), .up)
    }

    func testLeftAndRight() {
        XCTAssertEqual(classifier.classify(dx: -150, dy: 20), .left)
        XCTAssertEqual(classifier.classify(dx: 150, dy: -20), .right)
    }

    func testTooShortIsNone() {
        XCTAssertEqual(classifier.classify(dx: 10, dy: 59), .none)
    }

    func testExactlyMinDistanceCounts() {
        XCTAssertEqual(classifier.classify(dx: 0, dy: 60), .down)
    }

    func testDiagonalPrefersVertical() {
        // 縦と横が同じ量なら縦を優先する
        XCTAssertEqual(classifier.classify(dx: 100, dy: 100), .down)
    }

    func testLabels() {
        XCTAssertEqual(SwipeDirection.down.label, "下スワイプ")
        XCTAssertEqual(SwipeDirection.none.label, "スワイプなし")
    }
}
