import Foundation
import WebKit

@MainActor extension WKWebView {
    // Use the result callback explicitly. The completion-handler overload has a
    // default nil callback and can otherwise resolve to Void under Swift 5.
    func proofJavaScript(_ script: String, arguments: [String: Any]) async throws -> Any {
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Any, Error>) in
            callAsyncJavaScript(script, arguments: arguments, in: nil, in: .page) { result in
                continuation.resume(with: result)
            }
        }
    }
}
