# 轻舟 IPA iOS 0.4.0

本版针对 Apple 新登录协议重新接入认证与设备验证。保留 SwiftUI 玻璃界面、应用搜索、地区选择、第三方版本源、iPhone/iPad 版本、下载队列与分享导出。

## 认证与下载更新

- 客户端标识升级为 Configurator 2.21，按 Apple 动态配置使用 `/auth/v1/native` 等认证入口，兼容新旧配置结构。旧的 slash/bare 诊断选项已移除。
- 接入本地 Store 设备验证，处理 Apple 的 SP 注册挑战和 RP 重置；登录、免费许可领取、版本查询和下载请求刷新设备验证头。
- SAP 签名继续由固定版本 ipatool 的 Apple ARM64 框架和 TCI 解释器完成；设备验证使用参考 iPadn 的 WebKit/WASM 模块。无需用户为本应用开启 JIT。
- 仅在取得有效 Apple 会话令牌和账户标识后保存登录状态。环境检查包含动态配置、SAP 签名及本地设备组件初始化，不能代替实际账号登录或下载验证。
- Apple 密码不落盘。设备验证模块只接收设备标识、挑战与证明状态；会话保存在 iOS 钥匙串，第三方版本源不接收 Apple 登录数据。

## 安装与验证

IPA 为 arm64，最低 iOS 16。通过 LiveContainer 导入，或使用自己的有效证书重签名安装；构建的临时签名不能用于普通设备直接安装。

先确认账户页显示 **0.4.0**，点击“检查登录环境”，再使用自己的账号登录并验证一款已获许可的免费应用下载。首次需要联网下载并校验 SAP 组件，并初始化设备验证，请保持应用在前台。失败后可复制脱敏检查结果，不要发送密码、验证码、Cookie 或令牌。

本版的自动化验证不使用真实账号。Apple 账户认证、双重验证、购买许可、历史版本可用性及最终 IPA 下载，仍需在目标手机确认。没有绕过付费应用购买许可的功能。

## 来源与许可

- ipatool 基线：`majd/ipatool@d9c0247daea564d3ec7d04a9b015d0a29af66b04`。
- 新协议适配参考：`trikesh32/ipatool` 的 `fix/native-store-download@175c3e92a58a38fe538f4d94e785de4e575c6972`。
- Store 验证运行时与 plist 解析参考：`mengangs/ipadn@1ff9656c856cb29421a11e3de4816eee3251027a`。完整来源、原始组件声明、校验清单见 `third_party/ipadn-store-runtime/`。除 index.html 增加同源 CSP 外，运行时资源未修改。

组合后的 iOS 应用按 GPL-2.0-or-later 发布，见 `ios/COPYING`；保留各组件原有许可与来源说明。对应源码公开在 `ios-livecontainer` 分支及 Release 源码归档。

## 构建与检查

在 macOS 安装 Xcode 命令行工具、Go 1.26.5、CMake、Python 3、Node.js 后运行：

```sh
python3 ios/tests/verify_store_runtime.py
node ios/tests/store_runtime_smoke.cjs
bash ios/tests/test_store_webkit.sh
python3 ios/tests/test_native_http.py
bash ios/build-ipa.sh
```

产物：`dist/Qingzhou-IPA-iOS-0.4.0-NoJIT-arm64.ipa`。GitHub Actions 还运行 SAP、商店框架、HTTP 与应用商店适配测试，检查 IPA 结构和临时签名。WebKit 测试不使用 Apple 账号，验证实际引擎加载 WASM、同源限制与空设备状态；不等同于真机账号测试。
