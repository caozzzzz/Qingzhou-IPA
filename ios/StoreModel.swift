import SwiftUI
import CryptoKit

@MainActor final class StoreModel: ObservableObject {
    @Published var account: Account?
    @Published var accountBusy = false
    @Published var needsCode = false
    @Published var accountMessage = ""
    @Published var environmentMessage = ""
    @Published var searchMessage = ""
    @Published var searching = false
    @Published var apps: [CatalogApp] = []
    @Published var jobs: [DownloadJob] = []
    @Published var country = UserDefaults.standard.string(forKey: "country") ?? "us"
    @Published var platform = UserDefaults.standard.string(forKey: "platform") ?? "iphone"
    @Published var query = ""
    let countries: [[String]]
    private let storefronts: [String: String]
    private let auth = GoBridge(), catalog = GoBridge()
    private let guid: String
    private var pending: (email: String, password: String, remember: Bool, expires: Date)?
    private var authenticationGeneration = UUID(), searchGeneration = UUID()
    private var workers: [String: GoBridge] = [:], runs: [String: UUID] = [:]
    private var session: JSON?
    let folder: URL
    private let stateURL: URL

    init() {
        let data = Bundle.main.url(forResource: "regions", withExtension: "json").flatMap { try? Data(contentsOf: $0) }
        let regions = data.flatMap { try? JSONSerialization.jsonObject(with: $0) as? JSON } ?? [:]
        countries = regions["countries"] as? [[String]] ?? [["us", "美国"], ["cn", "中国"]]
        storefronts = regions["storefronts"] as? [String: String] ?? [:]
        let defaults = UserDefaults.standard
        guid = defaults.string(forKey: "guid") ?? ("02" + UUID().uuidString.replacingOccurrences(of: "-", with: "").prefix(10)).uppercased()
        defaults.set(guid, forKey: "guid")
        folder = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0].appendingPathComponent("Downloads", isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        stateURL = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0].appendingPathComponent("jobs.json")
        if let data = try? Data(contentsOf: stateURL), let saved = try? JSONDecoder().decode([DownloadJob].self, from: data) {
            jobs = saved.filter { UUID(uuidString: $0.id) != nil && $0.appID > 0 && $0.versionID.range(of: "^[1-9][0-9]{0,23}$", options: .regularExpression) != nil && ["iphone", "ipad"].contains($0.platform) }.map { j in
                var j = j; if j.status != "completed" || !FileManager.default.fileExists(atPath: folder.appendingPathComponent(j.fileName).path) { j.status = "paused" }; return j
            }
        }
        Task { await restore() }
    }
    private func base(_ action: String, _ fields: JSON = [:]) -> JSON { var json = fields; json["action"] = action; json["guid"] = guid; return json }
    private func restore() async {
        guard let data = SessionKeychain.load(), let saved = (try? JSONSerialization.jsonObject(with: data)) as? JSON, text(saved["guid"]) == guid, let stored = saved["session"] as? JSON else { return }
        accountBusy = true; defer { accountBusy = false }
        let result = await auth.request(base("restore", ["session": stored]))
        if result["ok"] as? Bool == true, let user = result["account"] as? JSON { account = Account(user); session = stored; accountMessage = "已恢复登录状态，下载时会由 Apple 验证。" }
        else { SessionKeychain.clear(); accountMessage = "无法恢复会话，请重新登录。" }
    }
    func login(email: String, password: String, remember: Bool, code: String = "") async {
        guard !accountBusy, account == nil, !email.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty, !password.isEmpty else { return }
        guard code.isEmpty || code.range(of: "^[0-9]{6}$", options: .regularExpression) != nil else { accountMessage = "手动验证码必须是六位数字。"; return }
        pending = nil; needsCode = false
        await authenticate(email: email.trimmingCharacters(in: .whitespacesAndNewlines), password: password, remember: remember, code: code)
    }
    func submitCode(_ code: String) async {
        guard !accountBusy, let saved = pending else { return }
        guard saved.expires > Date() else { cancelLogin(); accountMessage = "验证等待已过期，请重新输入密码。"; return }
        guard code.range(of: "^[0-9]{6}$", options: .regularExpression) != nil else { accountMessage = "请输入六位数字验证码。"; return }
        pending = nil
        await authenticate(email: saved.email, password: saved.password, remember: saved.remember, code: code)
    }
    private func authenticate(email: String, password: String, remember: Bool, code: String) async {
        accountBusy = true; accountMessage = "正在连接 Apple…"; let generation = authenticationGeneration
        let result = await auth.request(base("login", ["email": email, "password": password, "code": code]))
        guard generation == authenticationGeneration else { return }; accountBusy = false
        if result["need2FA"] as? Bool == true {
            pending = (email, password, remember, Date().addingTimeInterval(300)); needsCode = true; accountMessage = "请在受信任的设备上允许登录，并输入六位验证码。"
            Task { try? await Task.sleep(nanoseconds: 300_000_000_000); if let p = pending, p.expires <= Date() { cancelLogin(); accountMessage = "验证等待已过期。" } }
            return
        }
        needsCode = false; pending = nil
        guard result["ok"] as? Bool == true, let user = result["account"] as? JSON, var saved = result["session"] as? JSON else { accountMessage = text(result["error"]).isEmpty ? "Apple 未完成登录，请点击“检查登录环境”后重试。" : text(result["error"]); return }
        if var a = saved["account"] as? JSON { a.removeValue(forKey: "password"); saved["account"] = a }
        account = Account(user); session = saved; SessionKeychain.clear(); accountMessage = "已登录。"
        if remember {
            do { try SessionKeychain.save(JSONSerialization.data(withJSONObject: ["guid": guid, "session": saved])) }
            catch { accountMessage = "已登录，但无法保存会话；本次仅保留到退出。" }
        }
    }
    func checkEnvironment() async {
        guard !accountBusy else { return }
        let previousLoginMessage = accountMessage
        accountBusy = true; accountMessage = "正在检查认证配置、组件与本机解释器，首次检查可能需要几分钟…"
        let generation = authenticationGeneration
        let result = await auth.request(base("checkAuth"))
        guard generation == authenticationGeneration else { return }; accountBusy = false
        environmentMessage = result["ok"] as? Bool == true ? text((result["data"] as? JSON)?["message"]) : text(result["error"])
        accountMessage = previousLoginMessage
    }
    func cancelLogin() { authenticationGeneration = UUID(); pending = nil; needsCode = false; accountBusy = false; auth.cancel() }
    func logout() async {
        cancelLogin(); for job in jobs where workers[job.id] != nil { pause(job.id) }
        SessionKeychain.clear(); session = nil; account = nil; accountMessage = "已退出登录。"
        _ = await auth.request(base("logout"))
    }
    func search() async {
        guard !searching, !query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return }
        var region = country
        if region == "account" {
            guard let storefront = account?.storefront.components(separatedBy: "-").first, let resolved = storefronts[storefront] else { searchMessage = "请先登录，或手动选择商店地区。"; return }; region = resolved
        }
        UserDefaults.standard.set(country, forKey: "country"); UserDefaults.standard.set(platform, forKey: "platform")
        searching = true; apps = []; searchMessage = ""; let generation = UUID(); searchGeneration = generation
        let result = await catalog.request(base("search", ["query": query.trimmingCharacters(in: .whitespacesAndNewlines), "country": region, "platform": platform]))
        guard generation == searchGeneration else { return }; searching = false
        if result["ok"] as? Bool == true { apps = (result["data"] as? [JSON] ?? []).map(CatalogApp.init); searchMessage = apps.isEmpty ? "没有找到匹配的应用，试试 App ID 或其他地区。" : "查询结果 · \(apps.count) 款" }
        else { searchMessage = text(result["error"]) }
    }
    private func thirdParty(_ source: VersionSource, appID: Int64) async -> SourceResult {
        var result = SourceResult()
        let address: String
        switch source {
        case .timbrd: address = "https://api.timbrd.com/apple/app-version/index.php?id=\(appID)"
        case .bilin: address = "https://apis.bilin.eu.org/history/\(appID)"
        case .agzy: address = "https://app.agzy.cn/searchVersion?appid=\(appID)"
        default: return result
        }
        // This independent ephemeral session receives no Apple credentials or cookies.
        let config = URLSessionConfiguration.ephemeral; config.httpCookieStorage = nil; config.httpShouldSetCookies = false; config.timeoutIntervalForRequest = 20; config.timeoutIntervalForResource = 25
        let client = URLSession(configuration: config); defer { client.invalidateAndCancel() }
        do {
            var request = URLRequest(url: URL(string: address)!); request.setValue("application/json", forHTTPHeaderField: "Accept"); request.setValue("QingzhouIPA-iOS/0.1", forHTTPHeaderField: "User-Agent")
            let (data, response) = try await client.data(for: request)
            guard let response = response as? HTTPURLResponse, response.statusCode == 200, data.count <= 8 * 1024 * 1024, response.url?.host == URL(string: address)?.host else { throw URLError(.badServerResponse) }
            let value = try JSONSerialization.jsonObject(with: data)
            guard let list = (value as? [JSON]) ?? ((value as? JSON)?["data"] as? [JSON]) else { throw URLError(.cannotParseResponse) }
            var seen = Set<String>(); result.rows = list.prefix(20_000).compactMap { AppVersion.normalized($0, source: source.label) }.filter { seen.insert($0.id).inserted }
            result.diagnostics = ["\(source.label)：\(result.rows.count) 个"]
        } catch { result.diagnostics = ["\(source.label)：查询失败或超时"] }
        return result
    }
    private func appleVersions(appID: Int64, device: String, source: VersionSource) async -> SourceResult {
        guard account != nil else { return SourceResult(diagnostics: ["Apple：需先登录账户"] ) }
        let channel = source == .appleCombined ? "combined" : [.download, .compat, .redownload].contains(source) ? source.rawValue : "auto"
        let result = await auth.request(base("versions", ["appId": appID, "platform": device, "source": channel]))
        guard result["ok"] as? Bool == true, let data = result["data"] as? JSON else { return SourceResult(diagnostics: ["Apple：\(text(result["error"]))"]) }
        let latest = text(data["latestId"]); let ids = ([latest] + (data["ids"] as? [String] ?? [])).filter { $0.range(of: "^[1-9][0-9]{0,23}$", options: .regularExpression) != nil }
        var seen = Set<String>(); let rows = ids.filter { seen.insert($0).inserted }.map { id -> AppVersion in var row = AppVersion(id: id); row.sources = ["Apple"]; return row }
        return SourceResult(rows: rows, latestID: latest, diagnostics: ["Apple：\(rows.count) 个"])
    }
    func versions(app: CatalogApp, device: String, source: VersionSource) async -> SourceResult {
        var results: [SourceResult] = []
        if source == .combined {
            async let a = thirdParty(.timbrd, appID: app.id); async let b = thirdParty(.bilin, appID: app.id); async let c = thirdParty(.agzy, appID: app.id); async let d = appleVersions(appID: app.id, device: device, source: .apple)
            results = await [a, b, c, d]
        } else if source == .auto {
            for provider in [VersionSource.timbrd, .bilin, .agzy] { let r = await thirdParty(provider, appID: app.id); results.append(r); if !r.rows.isEmpty { break } }
            if results.allSatisfy({ $0.rows.isEmpty }) { results.append(await appleVersions(appID: app.id, device: device, source: .apple)) }
        } else if [.timbrd, .bilin, .agzy].contains(source) { results = [await thirdParty(source, appID: app.id)] }
        else { results = [await appleVersions(appID: app.id, device: device, source: source)] }
        var combined: [String: AppVersion] = [:]
        for row in results.flatMap({ $0.rows }) {
            if var old = combined[row.id] { old.sources = Array(Set(old.sources + row.sources)).sorted(); if old.version.isEmpty { old.version = row.version }; if old.updatedAt.isEmpty { old.updatedAt = row.updatedAt }; if old.sizeBytes == 0 { old.sizeBytes = row.sizeBytes }; combined[row.id] = old }
            else { combined[row.id] = row }
        }
        let latest = results.first(where: { !$0.latestID.isEmpty })?.latestID ?? combined.values.first(where: { $0.version == app.version })?.id ?? ""
        let rows = combined.values.sorted { a, b in
            if a.id == b.id { return false }; if a.id == latest { return true }; if b.id == latest { return false }
            return a.id.count == b.id.count ? a.id > b.id : a.id.count > b.id.count
        }
        return SourceResult(rows: rows, latestID: latest, diagnostics: results.flatMap { $0.diagnostics })
    }
    func metadata(appID: Int64, device: String, id: String) async -> JSON {
        guard account != nil else { return [:] }
        let result = await auth.request(base("metadata", ["appId": appID, "platform": device, "versionId": id])); return result["ok"] as? Bool == true ? result["data"] as? JSON ?? [:] : [:]
    }
    private var owner: String { SHA256.hash(data: Data((account?.email.lowercased() ?? "").utf8)).map { String(format: "%02x", $0) }.joined() }
    func enqueue(app: CatalogApp, device: String, version: AppVersion) -> String? {
        guard account != nil else { return "请先在账户页登录 Apple 账户。" }
        guard !jobs.contains(where: { $0.appID == app.id && $0.versionID == version.id && $0.platform == device && ["queued", "preparing", "downloading"].contains($0.status) }) else { return "该版本已在下载队列中。" }
        let job = DownloadJob(appID: app.id, name: app.name, platform: device, versionID: version.id, version: version.version, owner: owner, acquireLicense: app.price == 0)
        jobs.insert(job, at: 0); persist(); pump(); return nil
    }
    private func persist() {
        try? FileManager.default.createDirectory(at: stateURL.deletingLastPathComponent(), withIntermediateDirectories: true)
        if let data = try? JSONEncoder().encode(jobs) { try? data.write(to: stateURL, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication]) }
    }
    private func update(_ id: String, _ modify: (inout DownloadJob) -> Void) { if let index = jobs.firstIndex(where: { $0.id == id }) { modify(&jobs[index]) } }
    private func pump() {
        guard account != nil, workers.isEmpty, let job = jobs.first(where: { $0.status == "queued" && $0.owner == owner }) else { return }
        let worker = GoBridge(), generation = UUID(); workers[job.id] = worker; runs[job.id] = generation
        Task { await run(job: job, worker: worker, generation: generation) }
    }
    private func run(job: DownloadJob, worker: GoBridge, generation: UUID) async {
        UIApplication.shared.isIdleTimerDisabled = true
        let background = UIApplication.shared.beginBackgroundTask(withName: "IPA Download") { [weak self] in Task { @MainActor in self?.pause(job.id) } }
        defer {
            if background != .invalid { UIApplication.shared.endBackgroundTask(background) }
            if runs[job.id] == generation { runs.removeValue(forKey: job.id) }; if workers[job.id] === worker { workers.removeValue(forKey: job.id) }
            UIApplication.shared.isIdleTimerDisabled = !workers.isEmpty; persist(); pump()
        }
        update(job.id) { $0.status = "preparing"; $0.error = "" }; persist()
        let exported = await auth.request(base("export"))
        guard runs[job.id] == generation else { return }
        guard exported["ok"] as? Bool == true, let session = exported["session"] as? JSON else { update(job.id) { $0.status = "failed"; $0.error = "请重新登录。" }; return }
        let restored = await worker.request(base("restore", ["session": session]))
        guard runs[job.id] == generation else { return }
        guard restored["ok"] as? Bool == true else { update(job.id) { $0.status = "failed"; $0.error = "无法准备下载会话。" }; return }
        update(job.id) { $0.status = "downloading" }
        let polling = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 500_000_000)
                guard !Task.isCancelled, let self, self.runs[job.id] == generation else { break }
                if let progress = worker.poll()["data"] as? JSON { self.update(job.id) { $0.completedBytes = number(progress["bytes"]); $0.total = number(progress["total"]); $0.percent = min(0.999, max(0, (progress["percent"] as? NSNumber)?.doubleValue ?? 0)) } }
            }
        }
        let partial = folder.appendingPathComponent(job.partialName)
        let result = await worker.request(base("download", ["appId": job.appID, "platform": job.platform, "versionId": job.versionID, "outputPath": partial.path, "acquireLicense": job.acquireLicense]))
        polling.cancel(); guard runs[job.id] == generation else { return }
        if result["ok"] as? Bool == true, text((result["data"] as? JSON)?["path"]) == partial.path {
            do { try FileManager.default.moveItem(at: partial, to: folder.appendingPathComponent(job.fileName)); update(job.id) { $0.status = "completed"; $0.percent = 1; $0.completedBytes = number((result["data"] as? JSON)?["sizeBytes"]); $0.total = $0.completedBytes } }
            catch { update(job.id) { $0.status = "failed"; $0.error = "无法保存 IPA 文件。" } }
        } else { update(job.id) { $0.status = "failed"; $0.error = text(result["error"]).isEmpty ? "下载失败，请检查账户许可和网络。" : text(result["error"]) } }
    }
    func pause(_ id: String) { runs.removeValue(forKey: id); workers[id]?.cancel(); update(id) { if $0.status != "completed" { $0.status = "paused" } }; persist() }
    func resume(_ id: String) { guard let job = jobs.first(where: { $0.id == id }), job.owner == owner, account != nil else { accountMessage = "请登录创建该任务的账户后重试。"; return }; update(id) { $0.status = "queued"; $0.error = "" }; persist(); pump() }
    func remove(_ id: String) { pause(id); jobs.removeAll { $0.id == id }; persist() }
    func fileURL(_ job: DownloadJob) -> URL { folder.appendingPathComponent(job.fileName) }
}
