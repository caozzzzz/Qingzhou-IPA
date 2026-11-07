# 轻舟 IPA iOS 0.2.0 免 JIT 预览版

目标：iPhone / iPad、arm64、iOS 16.0 及以上。用于通过已有的自签工具或 LiveContainer 运行，不上架 App Store。

## 安装与运行

1. 在 GitHub Actions 的 `Build iOS IPA` 任务下载构建产物并解压得到 IPA。
2. 将 IPA 导入 LiveContainer，无需开启 JIT。
3. 先检查公开搜索和第三方历史版本查询，再尝试登录自己的 Apple 账户。
4. 下载时保持轻舟在前台；完成后在“下载”页分享或保存到“文件”。

产物使用 ad-hoc 签名，普通安装仍需由自己的有效证书重签名，或通过 LiveContainer 导入。0.2.0 将认证运行库改为固定版本的 Unicorn TCI 解释器，解释器缓冲区只申请读写内存，不再要求 JIT 权限。与 0.1.0 的 JIT 运行库不同。解释执行可能更慢，首次登录或环境检查需下载 Apple 认证组件。

账户页新增“检查登录环境”，不需要输入 Apple 账号或密码，可区分认证配置（BAG）、组件下载（ASSETS）、缓存目录（CACHE）和解释器（ENGINE）阶段的错误。手机网络仍可能影响登录；免 JIT 不代表绕过 Apple 的正常验证。

TrollStore 官方公开支持范围不包含 iOS 27.2。此预览版按 LiveContainer 或普通自签环境制作，不声称支持在 iOS 27.2 安装 TrollStore。

## 包含功能

- 手机原生 SwiftUI 界面，搜索、版本列表、账户、下载队列。
- 复用现有 Go / ipatool 后端；通过进程内 C 接口调用，无需启动 EXE 或额外服务。
- Timbrd、Bilin、Agzy、Apple 版本来源和合并去重。
- 登录、验证码、iOS 钥匙串保存会话；密码不持久化。
- 单任务下载、进度、暂停/续传、失败重试、IPA 校验和文件分享。
- 任务归属账户校验；重启后未完成任务暂停。

## 验证边界

CI 验证 arm64 iOS 编译和 IPA 结构、签名；既有 Go 测试覆盖复用的桥接逻辑。未接触用户 Apple 凭据，未在用户 iOS 27.2 设备上测试真实登录或 IPA 下载。LiveContainer 的宿主能力、网络、系统版本、Apple 接口和许可都可能影响实际结果。

## 在 Mac 上构建

需要 Xcode、Go 1.26.5 和 CMake：

```sh
bash ios/build-ipa.sh
```

产物：`dist/Qingzhou-IPA-iOS-0.2.0-NoJIT-arm64.ipa`。

构建脚本编译并静态链接 Unicorn TCI（mahee96/unicorn 的 a53ddc9ac6d65b24936d4a37917333fcd816cfd0 提交），校验上游归档 SHA-256，使用只读写缓冲区补丁，不再使用 JIT/W^X 切换。认证和下载仍受 Apple 账户许可约束。
