import Foundation
import WebKit
import SwiftUI
import Security

@MainActor final class StoreProofRuntime:NSObject,ObservableObject,WKNavigationDelegate {
    static let shared=StoreProofRuntime()
    @Published var presented=false
    @Published var web:WKWebView?
    @Published var status="正在准备设备验证…"
    private let server=StoreProofServer()
    private var busy=false
    private var navigation:CheckedContinuation<Void,Error>?
    private var activeControl:ProofTaskControl?
    private var generation=UUID()
    private let service="local.qingzhou.ipa.ios.store-proof"
    private func query(_ guid:String)->[String:Any] { [kSecClass as String:kSecClassGenericPassword,kSecAttrService as String:service,kSecAttrAccount as String:guid] }
    private func state(_ guid:String)->[String:Any] {
        var q=query(guid);q[kSecReturnData as String]=true;var value:CFTypeRef?
        if SecItemCopyMatching(q as CFDictionary,&value)==errSecSuccess,let data=value as? Data,let saved=(try? JSONSerialization.jsonObject(with:data)) as? [String:Any]{return saved}
        return ["identifier":String(UUID().uuidString.replacingOccurrences(of:"-",with:"").prefix(16)).lowercased(),"files":[String:String]()]
    }
    private func save(_ value:[String:Any],_ guid:String)throws {
        let data=try JSONSerialization.data(withJSONObject:value)
        let attributes:[String:Any]=[kSecValueData as String:data,kSecAttrAccessible as String:kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly]
        let result=SecItemUpdate(query(guid) as CFDictionary,attributes as CFDictionary)
        if result==errSecItemNotFound {var q=query(guid);attributes.forEach{q[$0.key]=$0.value};guard SecItemAdd(q as CFDictionary,nil)==errSecSuccess else {throw ProofFailure.unavailable}}
        else if result != errSecSuccess {throw ProofFailure.unavailable}
    }
    func cancel() {generation=UUID();activeControl?.cancel();navigation?.resume(throwing:CancellationError());navigation=nil;web?.navigationDelegate=nil;web?.stopLoading();web?.loadHTMLString("",baseURL:nil);web=nil;server.stop();presented=false}
    func cancel(_ control:ProofTaskControl) {control.cancel();if activeControl === control {cancel()}}
    func perform(_ request:[String:String],control:ProofTaskControl) async throws -> [String:Any] {
        let guid=request["guid"] ?? "",action=request["action"] ?? ""
        guard guid.range(of:"^[0-9A-Fa-f]{12}$",options:.regularExpression) != nil,["headers","provision","reset","check"].contains(action) else {throw ProofFailure.unavailable}
        while busy {try Task.checkCancellation();try await Task.sleep(nanoseconds:100000000)}
        try Task.checkCancellation()
        busy=true;activeControl=control;let id=generation;defer{busy=false;activeControl=nil}
        if action=="reset" {SecItemDelete(query(guid) as CFDictionary);return ["headers":[String:String]()]}
        let saved=state(guid)
        if action=="headers",(saved["files"] as? [String:String] ?? [:]).isEmpty {return ["headers":[String:String]()]}
        presented=true;status=action=="provision" ? "正在向 Apple 验证设备…" : "正在加载设备验证组件…"
        defer{web=nil;presented=false;server.stop()}
        let base=try await server.start()
        let config=WKWebViewConfiguration();config.websiteDataStore = .nonPersistent()
        let view=WKWebView(frame:.zero,configuration:config);view.navigationDelegate=self;web=view
        try await withCheckedThrowingContinuation { (continuation:CheckedContinuation<Void,Error>) in
            navigation=continuation;view.load(URLRequest(url:base.appendingPathComponent("runtime/index.html")))
        }
        try Task.checkCancellation();guard generation==id else {throw CancellationError()}
        let stateJSON=String(data:try JSONSerialization.data(withJSONObject:saved),encoding:.utf8)!
        let script=action=="provision" ? "return await sapStoreProvision(state, challenge, guid, {proxyURL: proxy});" : "return await sapStoreProofHeaders(state, {proxyURL: proxy});"
        let result=try await view.callAsyncJavaScript(script,arguments:["state":stateJSON,"challenge":request["challenge"] ?? "","guid":guid.uppercased(),"proxy":base.appendingPathComponent("proxy").absoluteString],in:nil,in:.page)
        try Task.checkCancellation();guard generation==id,let result=result as? String,let decoded=(try? JSONSerialization.jsonObject(with:Data(result.utf8))) as? [String:Any],let updated=decoded["state"] as? [String:Any] else {throw ProofFailure.unavailable}
        if action != "check" {try save(updated,guid)}
        let headers=decoded["headers"] as? [String:String] ?? [:]
        if action=="provision" {guard !(headers["X-Apple-AMD"] ?? "").isEmpty,!(headers["X-Apple-AMD-M"] ?? "").isEmpty else {throw ProofFailure.unavailable}}
        return ["headers":headers]
    }
    func webView(_ webView:WKWebView,didFinish navigation:WKNavigation!){self.navigation?.resume();self.navigation=nil}
    func webView(_ webView:WKWebView,didFail navigation:WKNavigation!,withError error:Error){self.navigation?.resume(throwing:ProofFailure.unavailable);self.navigation=nil}
    func webView(_ webView:WKWebView,didFailProvisionalNavigation navigation:WKNavigation!,withError error:Error){self.navigation?.resume(throwing:ProofFailure.unavailable);self.navigation=nil}
    func webViewWebContentProcessDidTerminate(_ webView:WKWebView){cancel()}
    func webView(_ webView:WKWebView,decidePolicyFor navigationAction:WKNavigationAction,decisionHandler:@escaping(WKNavigationActionPolicy)->Void){guard let target=navigationAction.request.url,target.scheme=="http",target.host=="127.0.0.1" else {decisionHandler(.cancel);return};decisionHandler(.allow)}
}
struct StoreProofView:View {
    @ObservedObject var runtime=StoreProofRuntime.shared
    var body:some View {VStack(spacing:20){ProgressView();Text(runtime.status);if let web=runtime.web {StoreProofWeb(web:web).frame(height:1).clipped()};Button("取消验证"){runtime.cancel()}}.padding(24).interactiveDismissDisabled()}
}
struct StoreProofWeb:UIViewRepresentable {let web:WKWebView;func makeUIView(context:Context)->WKWebView{web};func updateUIView(_ uiView:WKWebView,context:Context){}}

private final class ProofReply:@unchecked Sendable {
    let lock=NSLock();var data=Data("{\"error\":\"STORE_PROOF\"}".utf8);var finished=false
    func finish(_ reply:Data){lock.lock();defer{lock.unlock()};if !finished {data=reply;finished=true}}
    func value()->Data {lock.lock();defer{lock.unlock()};finished=true;return data}
}
final class ProofTaskControl:@unchecked Sendable {
    private let lock=NSLock()
    private var task:Task<Void,Never>?
    private var cancelled=false
    func attach(_ task:Task<Void,Never>) {lock.lock();defer{lock.unlock()};self.task=task;if cancelled {task.cancel()}}
    func cancel() {lock.lock();defer{lock.unlock()};cancelled=true;task?.cancel()}
}
// This ABI carries only a device identifier/challenge, never account data.
@_cdecl("QZStoreProof") func qzStoreProof(_ input:UnsafePointer<CChar>?)->UnsafeMutablePointer<CChar>? {
    guard !Thread.isMainThread,let input,let request=(try? JSONSerialization.jsonObject(with:Data(String(cString:input).utf8))) as? [String:String] else {return strdup("{\"error\":\"STORE_PROOF\"}")}
    let box=ProofReply(),done=DispatchSemaphore(value:0),control=ProofTaskControl()
    let task=Task {@MainActor in
        defer{done.signal()}
        do {let result=try await StoreProofRuntime.shared.perform(request,control:control);box.finish(try JSONSerialization.data(withJSONObject:result))}
        catch {box.finish(Data("{\"error\":\"STORE_PROOF\"}".utf8))}
    }
    control.attach(task)
    if done.wait(timeout:.now()+180) == .timedOut {control.cancel();Task {@MainActor in StoreProofRuntime.shared.cancel(control)}}
    return strdup(String(decoding:box.value(),as:UTF8.self))
}
