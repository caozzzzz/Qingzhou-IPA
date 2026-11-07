import SwiftUI
import UIKit

@main struct QingzhouApp: App {
    @StateObject private var model = StoreModel()
    var body: some Scene { WindowGroup { RootView().environmentObject(model) } }
}

struct FrostBackground: View {
    var body: some View { LinearGradient(colors: [Color(red: 0.88, green: 0.93, blue: 1), Color(red: 0.91, green: 0.96, blue: 0.95), Color(red: 0.94, green: 0.92, blue: 0.98)], startPoint: .topLeading, endPoint: .bottomTrailing).ignoresSafeArea() }
}
struct GlassButton: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View { configuration.label.font(.subheadline.weight(.semibold)).foregroundColor(Color(red: 0.15, green: 0.3, blue: 0.5)).padding(.horizontal, 17).padding(.vertical, 12).frame(minHeight: 44).background(Color(red: 0.86, green: 0.92, blue: 1).opacity(configuration.isPressed ? 1 : 0.85), in: RoundedRectangle(cornerRadius: 19, style: .continuous)).overlay(RoundedRectangle(cornerRadius: 19).stroke(Color.blue.opacity(0.2))).opacity(configuration.isPressed ? 0.8 : 1) }
}
extension View { func glassCard() -> some View { padding(17).background(.ultraThinMaterial, in: RoundedRectangle(cornerRadius: 24, style: .continuous)).overlay(RoundedRectangle(cornerRadius: 24).stroke(.white.opacity(0.8))) } }
struct AppArtwork: View {
    let app: CatalogApp
    var dimension: CGFloat = 60
    var body: some View {
        AsyncImage(url: app.imageURL) { phase in
            if let image = phase.image { image.resizable().scaledToFit() }
            else { RoundedRectangle(cornerRadius: dimension * 0.23).fill(Color.blue.opacity(0.12)).overlay(Text(String(app.name.prefix(1))).font(.title2).foregroundColor(.blue)) }
        }.frame(width: dimension, height: dimension).clipShape(RoundedRectangle(cornerRadius: dimension * 0.23, style: .continuous))
    }
}
struct RootView: View {
    var body: some View {
        TabView {
            SearchView().tabItem { Label("搜索", systemImage: "magnifyingglass") }
            DownloadsView().tabItem { Label("下载", systemImage: "arrow.down.circle") }
            AccountView().tabItem { Label("账户", systemImage: "person.crop.circle") }
        }.tint(Color(red: 0.24, green: 0.39, blue: 0.64))
    }
}
struct SearchView: View {
    @EnvironmentObject var model: StoreModel
    @State private var selected: CatalogApp?
    var body: some View {
        NavigationView {
            ZStack { FrostBackground(); ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    VStack(spacing: 12) {
                        TextField("应用名称或 App ID", text: $model.query).textInputAutocapitalization(.never).autocorrectionDisabled().submitLabel(.search).onSubmit { Task { await model.search() } }.padding(13).background(.white.opacity(0.7), in: RoundedRectangle(cornerRadius: 18))
                        Picker("设备", selection: $model.platform) { Text("iPhone").tag("iphone"); Text("iPad").tag("ipad") }.pickerStyle(.segmented)
                        HStack {
                            Picker("商店地区", selection: $model.country) { ForEach(model.countries, id: \.self) { region in Text(region[1]).tag(region[0]) } }.pickerStyle(.menu).frame(maxWidth: .infinity, alignment: .leading)
                            Button { Task { await model.search() } } label: { if model.searching { ProgressView().padding(.horizontal, 20) } else { Label("搜索", systemImage: "magnifyingglass") } }.buttonStyle(GlassButton()).disabled(model.searching || model.query.isEmpty)
                        }
                    }.glassCard()
                    if !model.searchMessage.isEmpty { Text(model.searchMessage).font(.subheadline).foregroundColor(.secondary) }
                    if model.searching { HStack { Spacer(); ProgressView("正在查询 Apple…"); Spacer() }.padding(25) }
                    LazyVStack(spacing: 15) { ForEach(model.apps) { app in
                        VStack(alignment: .leading, spacing: 16) {
                            HStack(alignment: .top, spacing: 13) { AppArtwork(app: app); VStack(alignment: .leading, spacing: 4) { Text(app.name).font(.headline).lineLimit(2); Text(app.developer).font(.caption).foregroundColor(.secondary).lineLimit(2) }; Spacer(minLength: 0) }
                            VStack(spacing: 7) { InfoLine(label: "App ID", value: String(app.id)); InfoLine(label: "最新版本", value: app.version); InfoLine(label: "更新时间", value: dateText(app.updatedAt)); InfoLine(label: "软件大小", value: bytes(app.sizeBytes)); InfoLine(label: "价格", value: app.priceText) }
                            Divider()
                            HStack { Text(app.platforms.map { $0 == "ipad" ? "iPad" : "iPhone" }.joined(separator: " / ")).font(.caption).foregroundColor(.secondary); Spacer(); Button("选择版本") { selected = app }.buttonStyle(GlassButton()) }
                        }.glassCard()
                    } }
                    Text("版本索引由所选来源提供，下载通过 Apple 进行。查询地区不会更改账户商店。").font(.caption).foregroundColor(.secondary).padding(.horizontal, 4)
                }.padding(16)
            } }.navigationTitle("轻舟 IPA").sheet(item: $selected) { app in VersionView(app: app, device: model.platform).environmentObject(model) }
        }.navigationViewStyle(.stack)
    }
}
struct InfoLine: View {
    let label, value: String
    var body: some View { HStack(alignment: .firstTextBaseline) { Text(label).foregroundColor(.secondary); Spacer(minLength: 8); Text(value.isEmpty ? "未提供" : value).multilineTextAlignment(.trailing).textSelection(.enabled) }.font(.caption) }
}
struct VersionView: View {
    @EnvironmentObject var model: StoreModel
    @Environment(\.dismiss) private var dismiss
    let app: CatalogApp
    let device: String
    @State private var source = VersionSource.auto
    @State private var result = SourceResult()
    @State private var loading = false
    @State private var selectedID = ""
    @State private var page = 0
    @State private var message = ""
    @State private var help = false
    @State private var generation = UUID()
    private var visible: [AppVersion] { Array(result.rows.dropFirst(page * 6).prefix(6)) }
    var body: some View {
        NavigationView {
            ZStack { FrostBackground(); VStack(spacing: 0) {
                VStack(alignment: .leading, spacing: 12) {
                    HStack(spacing: 13) { AppArtwork(app: app); VStack(alignment: .leading, spacing: 5) { Text(app.name).font(.headline).lineLimit(2); Text("App ID \(app.id) · \(device == "ipad" ? "iPad" : "iPhone")").font(.caption).foregroundColor(.secondary) }; Spacer(minLength: 0) }
                    Text("\(app.version) · \(bytes(app.sizeBytes)) · \(app.priceText)").font(.caption).foregroundColor(.secondary)
                }.padding(18).frame(maxWidth: .infinity, alignment: .leading).background(.ultraThinMaterial)
                ScrollView {
                    VStack(alignment: .leading, spacing: 15) {
                        HStack(spacing: 8) {
                            Text("查询通道").font(.subheadline)
                            Button { help = true } label: { Image(systemName: "questionmark.circle").frame(width: 30, height: 38) }.accessibilityLabel("查看查询通道说明")
                            Spacer(minLength: 0)
                            Picker("来源", selection: $source) { ForEach(VersionSource.allCases) { Text($0.label).tag($0) } }.pickerStyle(.menu).labelsHidden()
                        }
                        if loading { ProgressView("正在读取可用版本…").frame(maxWidth: .infinity).padding(30) }
                        else {
                            HStack { Text("选择版本").font(.headline); Spacer(); Text("共 \(result.rows.count) 个").font(.caption).foregroundColor(.secondary) }
                            Text(result.diagnostics.joined(separator: "；")).font(.caption).foregroundColor(.secondary)
                            if result.rows.isEmpty { Text("没有取得可用记录，请切换来源重试。").font(.subheadline).foregroundColor(.secondary); Button("重新查询") { Task { await refresh() } }.buttonStyle(GlassButton()) }
                            ForEach(visible) { version in
                                Button { selectedID = version.id } label: {
                                    HStack(alignment: .top, spacing: 12) {
                                        Image(systemName: selectedID == version.id ? "largecircle.fill.circle" : "circle").font(.title3).foregroundColor(.blue).padding(.top, 2)
                                        VStack(alignment: .leading, spacing: 5) {
                                            HStack { Text(version.version.isEmpty ? (version.id == result.latestID ? app.version : "版本号待查询") : version.version).font(.headline); if version.id == result.latestID { Text("最新").font(.caption2).padding(4).background(Color.blue.opacity(0.1), in: Capsule()) }; Spacer(); Text(version.displaySize).font(.caption) }
                                            Text("ID \(version.id)").font(.caption2).foregroundColor(.secondary)
                                            Text(dateText(version.updatedAt)).font(.caption).foregroundColor(.secondary)
                                            Text(version.dateSource == "third-party" ? "第三方记录 · \(version.sources.joined(separator: " / "))" : version.dateSource == "package-file-time" ? "包内文件时间" : "包内发布日期").font(.caption2).foregroundColor(.secondary)
                                        }
                                    }.foregroundColor(.primary).padding(14).background(selectedID == version.id ? Color.blue.opacity(0.09) : Color.white.opacity(0.6), in: RoundedRectangle(cornerRadius: 20))
                                }.buttonStyle(.plain)
                            }
                            if !result.rows.isEmpty { HStack { Button("上一页") { page -= 1 }.disabled(page == 0); Spacer(); Text("\(page + 1) / \(max(1, (result.rows.count + 5) / 6))").font(.caption); Spacer(); Button("下一页") { page += 1 }.disabled((page + 1) * 6 >= result.rows.count) }.buttonStyle(GlassButton()) }
                        }
                        if !message.isEmpty { Text(message).font(.caption).foregroundColor(.red) }
                    }.padding(16)
                }.task(id: visible.map(\.id)) { await readVisibleMetadata() }
                VStack(spacing: 9) {
                    Text("保存到轻舟的 Downloads，可在“下载”页分享至“文件”。").font(.caption2).foregroundColor(.secondary)
                    Button { enqueue() } label: { Label(model.account == nil ? "请先在账户页登录" : "下载所选版本", systemImage: "arrow.down.circle").frame(maxWidth: .infinity) }.buttonStyle(GlassButton()).disabled(loading || selectedID.isEmpty || model.account == nil)
                    Text("下载时请保持轻舟在前台。免费应用可获取免费许可。").font(.caption2).foregroundColor(.secondary)
                }.padding(16).background(.ultraThinMaterial)
            } }.navigationTitle("应用版本").navigationBarTitleDisplayMode(.inline).toolbar { ToolbarItem(placement: .navigationBarTrailing) { Button("完成") { dismiss() } } }
                .task { await refresh() }.onChange(of: source) { _ in Task { await refresh() } }.sheet(isPresented: $help) { SourceHelpView() }
        }.navigationViewStyle(.stack)
    }
    private func refresh() async {
        let token = UUID(); generation = token; loading = true; result = SourceResult(); selectedID = ""; page = 0; message = ""
        let response = await model.versions(app: app, device: device, source: source)
        guard generation == token else { return }; result = response; selectedID = response.rows.first?.id ?? ""; loading = false
    }
    private func readVisibleMetadata() async {
        let token = generation
        for row in visible where row.version.isEmpty && !row.metadataRead {
            guard !Task.isCancelled, generation == token else { return }
            let data = await model.metadata(appID: app.id, device: device, id: row.id)
            guard generation == token, let index = result.rows.firstIndex(where: { $0.id == row.id }) else { return }
            result.rows[index].metadataRead = true
            if !data.isEmpty { result.rows[index].version = text(data["version"]); result.rows[index].updatedAt = text(data["updatedAt"]); result.rows[index].sizeBytes = number(data["sizeBytes"]); result.rows[index].dateSource = text(data["dateSource"]) }
        }
    }
    private func enqueue() { guard let version = result.rows.first(where: { $0.id == selectedID }) else { return }; if let error = model.enqueue(app: app, device: device, version: version) { message = error } else { dismiss() } }
}
struct SourceHelpView: View {
    @Environment(\.dismiss) var dismiss
    var body: some View {
        NavigationView { ZStack { FrostBackground(); ScrollView { VStack(alignment: .leading, spacing: 16) {
            Text("无论选择哪个来源，实际 IPA 下载都通过 Apple 进行。第三方源提供历史版本记录，不能绕过付费应用的购买许可，也不能保证所有旧版本仍可下载。").font(.subheadline).glassCard()
            ForEach(VersionSource.allCases) { source in VStack(alignment: .leading, spacing: 7) { Text(source.label).font(.headline); Text(source.explanation).font(.subheadline).foregroundColor(.secondary) }.frame(maxWidth: .infinity, alignment: .leading).glassCard() }
        }.padding(16) } }.navigationTitle("查询通道说明").navigationBarTitleDisplayMode(.inline).toolbar { ToolbarItem(placement: .navigationBarTrailing) { Button("知道了") { dismiss() } } } }.navigationViewStyle(.stack)
    }
}
struct DownloadsView: View {
    @EnvironmentObject var model: StoreModel
    @State private var shared: SharedFile?
    let statuses = ["queued": "等待下载", "preparing": "准备下载", "downloading": "正在下载", "completed": "已完成", "failed": "下载失败", "paused": "已暂停"]
    var body: some View {
        NavigationView { ZStack { FrostBackground(); ScrollView { VStack(spacing: 16) {
            Text("下载时请保持轻舟在前台。应用重启后，未完成的任务会暂停。").font(.caption).foregroundColor(.secondary).frame(maxWidth: .infinity, alignment: .leading)
            if model.jobs.isEmpty { VStack(spacing: 12) { Image(systemName: "tray").font(.largeTitle); Text("还没有下载任务"); Text("搜索应用，选择版本后加入下载。").font(.caption).foregroundColor(.secondary) }.frame(maxWidth: .infinity).padding(40).glassCard() }
            ForEach(model.jobs) { job in VStack(alignment: .leading, spacing: 10) {
                Text(job.name).font(.headline); Text("\(job.platform == "ipad" ? "iPad" : "iPhone") · \(job.version.isEmpty ? job.versionID : job.version)").font(.caption).foregroundColor(.secondary)
                if job.status == "preparing" { ProgressView() } else if job.status == "downloading" { ProgressView(value: job.percent) }
                Text("\(statuses[job.status] ?? job.status) · \(bytes(job.completedBytes))\(job.total > 0 ? " / " + bytes(job.total) : "")").font(.caption).foregroundColor(.secondary)
                if !job.error.isEmpty { Text(job.error).font(.caption).foregroundColor(.red) }
                HStack {
                    if job.status == "completed" { Button("分享 / 保存到文件") { shared = SharedFile(url: model.fileURL(job)) } }
                    else if ["queued", "preparing", "downloading"].contains(job.status) { Button("暂停") { model.pause(job.id) } }
                    else { Button("继续 / 重试") { model.resume(job.id) } }
                    Spacer(); Button("移除记录") { model.remove(job.id) }
                }.buttonStyle(GlassButton())
            }.glassCard() }
            Text("移除记录不会删除已下载文件。").font(.caption2).foregroundColor(.secondary)
        }.padding(16) } }.navigationTitle("下载队列").sheet(item: $shared) { ShareSheet(url: $0.url) } }.navigationViewStyle(.stack)
    }
}
struct SharedFile: Identifiable { let id = UUID(); let url: URL }
struct ShareSheet: UIViewControllerRepresentable {
    let url: URL
    func makeUIViewController(context: Context) -> UIActivityViewController { UIActivityViewController(activityItems: [url], applicationActivities: nil) }
    func updateUIViewController(_ uiViewController: UIActivityViewController, context: Context) {}
}
struct AccountView: View {
    @EnvironmentObject var model: StoreModel
    @State private var email = ""
    @State private var password = ""
    @State private var code = ""
    @State private var remember = false
    @State private var manualVerification = false
    @State private var manualCode = ""
    var body: some View {
        NavigationView { ZStack { FrostBackground(); ScrollView { VStack(alignment: .leading, spacing: 18) {
            VStack(alignment: .leading, spacing: 10) { Label("免 JIT 运行", systemImage: "iphone").font(.headline); Text("登录认证改为本机解释执行，无需开启 JIT。首次登录可能需要下载认证组件，请保持联网并将应用留在前台。").font(.caption).foregroundColor(.secondary) }.glassCard()
            VStack(alignment: .leading, spacing: 14) {
                if let account = model.account {
                    Label(account.name.isEmpty ? account.email : account.name, systemImage: "person.crop.circle.fill").font(.headline)
                    Text(account.email).font(.subheadline).textSelection(.enabled)
                    Text("账户商店：\(account.storefront)").font(.caption).foregroundColor(.secondary)
                    Button("退出登录") { Task { await model.logout() } }.buttonStyle(GlassButton()).disabled(model.accountBusy)
                } else if model.needsCode {
                    Text("输入六位验证码").font(.headline)
                    TextField("验证码", text: $code).keyboardType(.numberPad).textContentType(.oneTimeCode).padding(13).background(.white.opacity(0.7), in: RoundedRectangle(cornerRadius: 18))
                    Button("验证并登录") { let value = code; code = ""; Task { await model.submitCode(value) } }.buttonStyle(GlassButton()).disabled(model.accountBusy)
                    Button("取消验证") { model.cancelLogin(); clearCredentials() }
                } else {
                    Text("Apple 账户").font(.headline)
                    TextField("账号邮箱", text: $email).keyboardType(.emailAddress).textContentType(.username).textInputAutocapitalization(.never).autocorrectionDisabled().padding(13).background(.white.opacity(0.7), in: RoundedRectangle(cornerRadius: 18))
                    SecureField("密码", text: $password).disabled(model.accountBusy).textContentType(.password).padding(13).background(.white.opacity(0.7), in: RoundedRectangle(cornerRadius: 18))
                    DisclosureGroup("手动输入双重验证码", isExpanded: $manualVerification) {
                        VStack(alignment: .leading, spacing: 8) {
                            TextField("六位验证码", text: $manualCode).keyboardType(.numberPad).textContentType(.oneTimeCode).padding(13).background(.white.opacity(0.7), in: RoundedRectangle(cornerRadius: 18))
                            Text("可填写受信任 Apple 设备上取得的新验证码；密码框只填账户密码，不要重复拼接验证码。").font(.caption).foregroundColor(.secondary)
                        }.padding(.top, 8)
                    }
                    Toggle("记住登录状态", isOn: $remember)
                    Button { let value = password; let verification = manualVerification ? manualCode : ""; manualCode = ""; Task { await model.login(email: email, password: value, remember: remember, code: verification) } } label: { HStack { if model.accountBusy { ProgressView() }; Text(model.accountBusy ? "正在处理…" : "登录") }.frame(maxWidth: .infinity) }.buttonStyle(GlassButton()).disabled(model.accountBusy || password.isEmpty || email.isEmpty)
                    Button("检查登录环境") { Task { await model.checkEnvironment() } }.buttonStyle(GlassButton()).disabled(model.accountBusy)
                    if model.accountBusy { Button("取消登录") { model.cancelLogin(); clearCredentials() } }
                }
                if !model.environmentMessage.isEmpty { Text("环境检查：" + model.environmentMessage).font(.caption).foregroundColor(.secondary).textSelection(.enabled) }
                if !model.accountMessage.isEmpty { Text(model.accountMessage).font(.caption).foregroundColor(.secondary).textSelection(.enabled) }
                if !model.environmentMessage.isEmpty || !model.accountMessage.isEmpty {
                    Button("复制检查结果") { UIPasteboard.general.string = "轻舟 iOS 0.2.2\n环境检查：\(model.environmentMessage)\n登录结果：\(model.accountMessage)" }.font(.caption)
                }
            }.glassCard()
            Text("记住登录时，会话保存到 iOS 钥匙串，不保存 Apple 密码。第三方版本源仅收到公开 App ID。").font(.caption).foregroundColor(.secondary)
            Text("轻舟 IPA iOS · 0.2.2 免 JIT 预览版\n登录和下载需要在目标设备上验证。").font(.caption2).foregroundColor(.secondary)
        }.padding(16) } }.navigationTitle("账户") }.navigationViewStyle(.stack)
        .onChange(of: model.account?.email) { accountEmail in
            if accountEmail != nil { clearCredentials() }
        }
    }
    private func clearCredentials() { password = ""; code = ""; manualCode = "" }
}
