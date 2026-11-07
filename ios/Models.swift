import Foundation
import Security

typealias JSON = [String: Any]
func number(_ value: Any?) -> Int64 { (value as? NSNumber)?.int64Value ?? Int64(value as? String ?? "") ?? 0 }
func text(_ value: Any?) -> String { value as? String ?? (value as? NSNumber)?.stringValue ?? "" }
func bytes(_ value: Int64) -> String { value > 0 ? ByteCountFormatter.string(fromByteCount: value, countStyle: .decimal) : "未提供" }
func dateText(_ value: String) -> String {
    guard !value.isEmpty else { return "未提供" }
    if let date = ISO8601DateFormatter().date(from: value) { return date.formatted(date: .numeric, time: .shortened) }
    return value
}

struct CatalogApp: Identifiable {
    let id: Int64
    let name, developer, version, updatedAt, artwork, currency, minimumOS: String
    let sizeBytes: Int64
    let price: Double?
    let platforms: [String]
    init(_ json: JSON) {
        id = number(json["id"]); name = text(json["name"]); developer = text(json["developer"])
        version = text(json["version"]); updatedAt = text(json["updatedAt"]); artwork = text(json["artwork"])
        currency = text(json["currency"]); minimumOS = text(json["minimumOS"]); sizeBytes = number(json["sizeBytes"])
        price = (json["price"] as? NSNumber)?.doubleValue; platforms = json["platforms"] as? [String] ?? []
    }
    var priceText: String {
        guard let price else { return "未提供" }; if price == 0 { return "免费" }
        guard !currency.isEmpty else { return String(price) }
        let f = NumberFormatter(); f.numberStyle = .currency; f.currencyCode = currency
        return f.string(from: NSNumber(value: price)) ?? "\(price) \(currency)"
    }
    var imageURL: URL? {
        guard let url = URL(string: artwork), url.scheme == "https", url.host?.hasSuffix(".mzstatic.com") == true else { return nil }; return url
    }
}

struct AppVersion: Identifiable {
    let id: String
    var version = "", updatedAt = "", sizeBytes: Int64 = 0, sizeText = "", sources: [String] = [], dateSource = "third-party", metadataRead = false
    var displaySize: String { sizeBytes > 0 ? bytes(sizeBytes) : sizeText.isEmpty ? "未提供" : sizeText }
    static func normalized(_ item: JSON, source: String) -> AppVersion? {
        let raw = item["external_identifier"] ?? item["versionId"] ?? item["version_id"] ?? item["id"]
        if let n = raw as? NSNumber, (n.doubleValue.rounded() != n.doubleValue || n.doubleValue > 9_007_199_254_740_991) { return nil }
        let id = text(raw).trimmingCharacters(in: .whitespacesAndNewlines)
        let version = text(item["bundle_version"] ?? item["version"] ?? item["bundleShortVersionString"]).components(separatedBy: .newlines).first ?? ""
        guard id.range(of: "^[1-9][0-9]{0,23}$", options: .regularExpression) != nil, !version.isEmpty, version.count <= 64, version.rangeOfCharacter(from: .decimalDigits) != nil else { return nil }
        var row = AppVersion(id: id); row.version = version; row.sources = [source]
        row.updatedAt = String(text(item["created_at"] ?? item["createTime"] ?? item["updateTime"] ?? item["date"] ?? item["time"]).prefix(100))
        let size = item["size"] ?? item["fileSize"] ?? item["fileSizeBytes"]
        row.sizeBytes = max(0, number(size)); if row.sizeBytes == 0 { row.sizeText = String(text(size).prefix(40)) }
        return row
    }
}

enum VersionSource: String, CaseIterable, Identifiable {
    case auto, combined, timbrd, bilin, agzy, apple, appleCombined = "apple-combined", download, compat, redownload
    var id: String { rawValue }
    var label: String { switch self {
    case .auto: return "自动（第三方优先）"; case .combined: return "合并全部来源"
    case .timbrd: return "Timbrd"; case .bilin: return "Bilin"; case .agzy: return "Agzy"
    case .apple: return "Apple 自动"; case .appleCombined: return "Apple 通道合并"
    case .download: return "Apple 下载接口"; case .compat: return "Apple 兼容接口"; case .redownload: return "Apple 重下载接口"
    } }
    var explanation: String { switch self {
    case .auto: return "依次查询 Timbrd → Bilin → Agzy，第一个有记录就返回；都没有结果再查询 Apple。"
    case .combined: return "查询三个第三方源及 Apple 自动通道，合并记录、按版本 ID 去重。"
    case .timbrd, .bilin, .agzy: return "只查 \(label) 的历史版本数据库。"
    case .apple: return "只向 Apple 查询，由底层程序自动选择、回退请求通道。"
    case .appleCombined: return "分别查询三个 Apple 通道，再合并去重。"
    case .download: return "使用 Apple 下发的下载地址和设备验证流程，提取返回的版本列表。"
    case .compat: return "使用旧式 volumeStore 下载请求，作为另一种查询方式。"
    case .redownload: return "先取得当前版本 ID，再通过重新下载请求查询。"
    } }
}

struct SourceResult { var rows: [AppVersion] = []; var latestID = ""; var diagnostics: [String] = [] }
struct Account { let email, name, storefront: String; init(_ j: JSON) { email = text(j["email"]); name = text(j["name"]); storefront = text(j["storefront"]) } }

struct DownloadJob: Identifiable, Codable {
    var id = UUID().uuidString
    var appID: Int64, name: String, platform: String, versionID: String, version: String, owner: String, acquireLicense: Bool
    var status = "queued", error = "", percent = 0.0, completedBytes: Int64 = 0, total: Int64 = 0
    var fileName: String { "\(appID)_\(platform)_\(versionID)_\(id.prefix(8)).ipa" }
    var partialName: String { ".qingzhou-\(id).staging" }
}

final class GoBridge {
    private let handle: UInt64 = QZCreate()
    private let queue = DispatchQueue(label: "Qingzhou.GoBridge.\(UUID())", qos: .userInitiated)
    init() {
        if let path = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask).first?.path {
            _ = path.withCString { QZSetCacheDirectory(UnsafeMutablePointer(mutating: $0)) }
        }
    }
    func request(_ input: JSON) async -> JSON {
        guard let data = try? JSONSerialization.data(withJSONObject: input), let payload = String(data: data, encoding: .utf8) else { return ["ok": false, "error": "请求无效。"] }
        return await withCheckedContinuation { continuation in
            queue.async { [self] in
                let result = payload.withCString { QZCall(handle, UnsafeMutablePointer(mutating: $0)) }
                continuation.resume(returning: decode(result))
            }
        }
    }
    private func decode(_ pointer: UnsafeMutablePointer<CChar>?) -> JSON {
        guard let pointer else { return ["ok": false, "error": "服务未返回数据。"] }; defer { QZFree(pointer) }
        let data = Data(String(cString: pointer).utf8)
        return (try? JSONSerialization.jsonObject(with: data)) as? JSON ?? [:]
    }
    func poll() -> JSON { decode(QZPoll(handle)) }
    func cancel() { QZCancel(handle) }
    deinit { QZClose(handle) }
}

enum SessionKeychain {
    static let service = "local.qingzhou.ipa.ios.session"
    static var query: [String: Any] { [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: "apple-session"] }
    static func load() -> Data? { var q = query; q[kSecReturnData as String] = true; var value: CFTypeRef?; return SecItemCopyMatching(q as CFDictionary, &value) == errSecSuccess ? value as? Data : nil }
    static func save(_ data: Data) throws {
        clear(); var q = query; q[kSecValueData as String] = data; q[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        guard SecItemAdd(q as CFDictionary, nil) == errSecSuccess else { throw NSError(domain: "QingzhouKeychain", code: 1) }
    }
    static func clear() { SecItemDelete(query as CFDictionary) }
}
