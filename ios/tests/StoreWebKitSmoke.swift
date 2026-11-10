import AppKit
import WebKit
import Foundation

@MainActor final class SmokeNavigation: NSObject, WKNavigationDelegate {
    var waiting: CheckedContinuation<Void, Error>?
    func load(_ web: WKWebView, _ url: URL) async throws {
        web.navigationDelegate = self
        try await withCheckedThrowingContinuation { continuation in
            waiting = continuation
            web.load(URLRequest(url: url))
        }
    }
    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) { waiting?.resume(); waiting = nil }
    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) { waiting?.resume(throwing: ProofFailure.unavailable); waiting = nil }
    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) { waiting?.resume(throwing: ProofFailure.unavailable); waiting = nil }
}

@main struct StoreWebKitSmoke {
    @MainActor static func check() async throws {
        // Exercise the production parser and loopback server, with no Apple account.
        let envelope = Data("<Document><Protocol><key>status</key><integer>0</integer></Protocol></Document>".utf8)
        guard (try ProofPropertyList.dictionary(envelope))["status"] as? Int == 0 else { throw ProofFailure.unavailable }
        do { _ = try ProofPropertyList.dictionary(Data("<html><dict/></html>".utf8)); throw ProofFailure.unavailable }
        catch is ProofPropertyListError {}
        let server = StoreProofServer()
        let base = try await server.start()
        defer { server.stop() }
        for target in ["proxy?url=https://example.com/", "runtime/../Info.plist"] {
            var request = URLRequest(url: base.appendingPathComponent(target.components(separatedBy: "?")[0]))
            if target.contains("?") { request.url = URL(string: base.absoluteString + target); request.httpMethod = "POST" }
            let (_, response) = try await URLSession.shared.data(for: request)
            guard let response = response as? HTTPURLResponse, [403,404].contains(response.statusCode) else { throw ProofFailure.unavailable }
        }
        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .nonPersistent()
        let web = WKWebView(frame: NSRect(x: 0, y: 0, width: 200, height: 200), configuration: configuration)
        let window = NSWindow(contentRect: web.frame, styleMask: [.borderless], backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false
        window.contentView = web
        let navigation = SmokeNavigation()
        try await navigation.load(web, base.appendingPathComponent("runtime/index.html"))
        let raw = try await web.callAsyncJavaScript("return await sapStoreProofHeaders(state, {proxyURL: proxy});", arguments: ["state":"{\"identifier\":\"0123456789abcdef\",\"files\":{}}", "proxy":base.appendingPathComponent("proxy").absoluteString], in: nil, in: .page)
        guard let raw = raw as? String, let value = try JSONSerialization.jsonObject(with: Data(raw.utf8)) as? [String:Any], value["provisioned"] as? Bool == false, let state = value["state"] as? [String:Any], state["identifier"] as? String == "0123456789abcdef" else { throw ProofFailure.unavailable }
        // CSP must prevent JavaScript from contacting any remote server directly.
        let blocked = try await web.callAsyncJavaScript("try { await fetch('https://example.com/'); return false; } catch (_) { return true; }", arguments: [:], in: nil, in: .page)
        guard blocked as? Bool == true else { throw ProofFailure.unavailable }
        print("PASS: production parser, loopback restrictions, WebKit WASM runtime and remote-fetch CSP; no Apple account used")
        window.close()
    }
    static func main() {
        let app = NSApplication.shared
        app.setActivationPolicy(.prohibited)
        DispatchQueue.main.asyncAfter(deadline: .now() + 150) { fputs("WebKit smoke timed out\n", stderr); exit(1) }
        Task { @MainActor in
            do { try await check(); exit(0) }
            catch { fputs("WebKit smoke failed: component/parser/server error\n", stderr); exit(1) }
        }
        app.run()
    }
}
