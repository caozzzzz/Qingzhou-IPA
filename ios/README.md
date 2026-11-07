# 轻舟 IPA iOS 预览版

目标：iPhone / iPad、arm64、iOS 16.0 及以上。用于通过已有的自签工具或 LiveContainer 运行，不上架 App Store。

## 安装与运行

1. 在 GitHub Actions 的 `Build iOS IPA` 任务下载构建产物并解压得到 IPA。
2. 将 IPA 导入 LiveContainer，并为轻舟开启 JIT。
3. 先检查公开搜索和第三方历史版本查询，再尝试登录自己的 Apple 账户。
4. 下载时保持轻舟在前台；完成后在“下载”页分享或保存到“文件”。

产物使用 ad-hoc 签名，不是可直接通过 App Store / 系统安装的发行签名。普通自签工具需使用自己的有效签名。搜索和第三方版本查询不依赖 JIT，但现有 Apple 登录认证使用 Unicorn 运行认证代码，需要支持 JIT 的运行环境。仅有签名不代表已具备 JIT，普通自签下不保证登录和下载可用。

TrollStore 官方公开支持范围不包含 iOS 27.2。此预览版按用户已有的 LiveContainer + JIT 环境制作，不声称支持在 iOS 27.2 安装 TrollStore。

## 包含功能

- 手机原生 SwiftUI 界面，搜索、版本列表、账户、下载队列。
- 复用现有 Go / ipatool 后端；通过进程内 C 接口调用，无需启动 EXE 或额外服务。
- Timbrd、Bilin、Agzy、Apple 版本来源和合并去重。
- 登录、验证码、iOS 钥匙串保存会话；密码不持久化。
- 单任务下载、进度、暂停/续传、失败重试、IPA 校验和文件分享。
- 任务归属账户校验；重启后未完成任务暂停。

## 验证边界

CI 验证 arm64 iOS 编译和 IPA 结构、签名；既有 Go 测试覆盖复用的桥接逻辑。未接触用户 Apple 凭据，未在用户 iOS 27.2 设备上测试真实登录或 IPA 下载。LiveContainer 的宿主能力、JIT、网络、系统版本、Apple 接口和许可都可能影响实际结果。

## 在 Mac 上构建

需要 Xcode、Go 1.26.5 和 CMake：

```sh
bash ios/build-ipa.sh
```

产物：`dist/Qingzhou-IPA-iOS-0.1.0-arm64.ipa`。

构建脚本编译并静态链接 Unicorn 2.1.4，校验上游归档 SHA-256，使用现有 ipatool iOS W^X 补丁。认证和下载仍受 Apple 账户许可约束。
