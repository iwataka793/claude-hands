import XCTest

/// クラウドのシミュレーターで、Claude が実際にスワイプして確かめるためのテスト。
/// 各段階のスクリーンショットを残す（Actions の成果物として取り出す）。
final class SwipeUITests: XCTestCase {
    override func setUp() {
        continueAfterFailure = false
    }

    func testSwipeDownAddsLine() {
        let app = XCUIApplication()
        app.launch()
        attachScreenshot(app, name: "01-launch")

        let area = app.descendants(matching: .any)["swipeArea"]
        XCTAssertTrue(area.waitForExistence(timeout: 10), "スワイプする場所が見つからない")

        area.swipeDown()
        attachScreenshot(app, name: "02-after-swipe-down")
        XCTAssertTrue(app.staticTexts.containing(NSPredicate(format: "label BEGINSWITH %@", "下スワイプ")).firstMatch
            .waitForExistence(timeout: 5), "下スワイプの行が出ない")

        area.swipeUp()
        attachScreenshot(app, name: "03-after-swipe-up")
        XCTAssertTrue(app.staticTexts.containing(NSPredicate(format: "label BEGINSWITH %@", "上スワイプ")).firstMatch
            .waitForExistence(timeout: 5), "上スワイプの行が出ない")
    }

    private func attachScreenshot(_ app: XCUIApplication, name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }
}
