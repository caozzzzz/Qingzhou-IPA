# 轻舟 IPA App Store 下载器 v0.8.1

使用 ipatool v2.7.0 Go 认证库真实登录 App Store。支持六位验证码、取消、账户信息、退出和可选加密会话保存。
支持 Apple 实时名称/App ID 搜索、iPhone/iPad 筛选、目录更新时间和大小、真实版本列表/元数据、真实 IPA 下载、暂停继续、完成库及目录设置。

## 开发与打包
需要 Node.js 22+、Go 1.26.5（或兼容工具链）、Windows x64。

```powershell
npm ci
npm run build:bridge
npm start
npm run dist
```

打包产物位于 dist/。
GitHub 源码不包含编译后的认证桥，首次启动前请执行 npm run build:bridge。

## 验证
```powershell
npm test
cd authbridge
go test ./...
cd ..
npx electron tests/auth-ui-check.cjs
npx electron tests/store-ui-check.cjs
npx electron . --smoke-test --live-catalog --report=../desktop-smoke.json
```

UI 测试使用虚构 .invalid 账户，不联系 Apple。桌面检查使用真正的 Go 桥执行 ping、输入校验及 Windows 加密往返验证，不发送登录凭据。
未使用用户真实账户进行端到端认证。实际登录结果以 Apple 返回为准。

## 数据处理
密码和验证码通过本机管道传递，不写进程参数、日志或磁盘。
等待双重认证时密码最多保留 5 分钟；取消或退出会清除引用并终止认证进程。
JavaScript/Go 管理内存，清除引用不保证物理内存立即擦除。
默认不记住登录。选择记住后，Electron safeStorage 使用 Windows DPAPI 加密会话。
数据位置为当前用户 %APPDATA%/QingzhouIPA/apple-session.encrypted，不包含 Apple 账户密码。
加密不可用时不降级为明文存储；无法恢复时提示重新登录。
本机恢复不代表 Apple 已重新验证。会话过期需重新登录。
退出清除本机桥接进程及已保存会话，不影响其他设备的 Apple 登录。
渲染页面启用沙箱、上下文隔离和 CSP，仅开放五个认证接口，并在主进程检查请求来源。
本地构建未签名，不自动更新。

依赖许可见 THIRD-PARTY-NOTICES.txt、third-party-licenses，以及 Electron 组件自带的许可文件。

## 搜索与下载实现
公共搜索使用 Apple iTunes Search/Lookup API，名称走 search，数字 App ID 走 lookup。
下载与历史版本查询使用 ipatool v2.7.0，版本信息通过 HTTP Range 读取真实 IPA。
third_party/ipatool 是 MIT 许可的本地源副本；局部扩展公开 IPA 长度与日期来源，修改见 QINGZHOU-PATCH.md。
下载任务从当前会话创建独立桥接进程，不将令牌发送到渲染页面。仅在用户明确点击时尝试获取免费应用许可，不购买付费应用。
下载先写入临时文件，经 ipatool 处理、SINF 写入及应用/版本/设备族校验后，才重命名为 .ipa 并标记完成。
任务与偏好保存在当前用户数据目录；临时文件支持服务器允许的 Range 续传。移除记录不删除文件。
版本目录更新时间来自 Apple；包内文件时间明确标注，不冒充商店发布日期。

本次实际 Apple 名称与 App ID 搜索已验证；认证下载通过本地逻辑、HTTP 元数据及界面测试，未使用用户账号完成实际 IPA 下载。
旧版本可用性、账号所属商店、许可、会话及 Apple 服务响应会影响结果。工具不绕过 Apple 许可和加密。

## v0.4.0
界面去除宣传式文案，版本标签只显示 v0.4.0。regions.js 提供 25 个地区及随账号选项，并复用 ipatool storefront 对照表解析账户地区。
选择单次保存目录时，通过主进程原生目录对话框取得路径，返回进程内授权 token；渲染页面不直接提交任意路径。任务目录与默认目录分开维护。
brandmark.svg、icon.ico 为本项目绘制的帆船图标；Windows 资源编辑已启用，关闭的是签名操作，不再跳过图标写入。

## v0.5.0 界面
使用 glass.css 统一雾面玻璃背景、半透明面板、圆角、边框与阴影。
搜索卡片统一图标、标题/开发者预留区域、信息行及按钮位置，长文本两行截断并保留完整提示。
等待状态使用转圈与骨架占位，支持系统减少动画偏好。
版本窗口由固定应用头部、独立滚动列表、固定目录/下载操作组成；元数据更新会保留滚动和键盘焦点。
新增 tests/glass-ui-check.cjs 检查对齐、加载动画、固定区域、圆角裁切及异步更新后的滚动位置。

## v0.6.0 动态更新修复
v0.5 的整窗 innerHTML 重建被移除：应用头部和滚动容器保留，仅修改完成查询的版本行，不重置其他行的动画或图标节点。
列表与操作区分开更新，并避免给保留的分页按钮重复注册事件。
卡片使用固定三段网格，设备标签限制为两行；小窗口/大缩放压缩头部及底部占用，窗口初始大小适配屏幕工作区。
新增 tests/layout-matrix-check.cjs，可用 --root=已解包目录 对发行包文件执行同样检查。
该检查测量实际动画变换前后差异、头部及等待节点保留、滚轮输入和 100%/150%/200% 缩放，而非仅检查 CSS 声明。


## v0.7–v0.8.1

- 查询结果与版本详情显示价格、币种；应用图标为接近 iPhone 的圆角效果。
- 主要按钮采用浅蓝色玻璃底与深色文字，按钮、输入框和下拉框统一增加圆角。
- 新增 Timbrd、Bilin、Agzy 历史版本源，支持单独查询及与 Apple 返回的版本 ID 合并去重，显示来源数量和失败状态。
- 自动模式依次尝试 Timbrd、Bilin、Agzy，第一个有记录即返回；均无记录时再查询 Apple。
- Apple 自动模式沿用 ipatool 回退，Apple 通道合并分别查询下载、volumeStore 兼容、重下载三个通道。
- 修正数字版本 ID 可能被转换成科学计数法的问题。
- 查询通道旁的小问号显示下载许可说明及全部 10 个选项的对照表，关闭后保留版本选择与焦点。

第三方源只提供历史版本记录，实际 IPA 下载始终通过 Apple 进行，不能绕过付费应用的购买许可，也不能保证所有旧版本仍可下载。第三方请求只携带公开 App ID，不携带 Apple 账号、密码、会话、Cookie 或设备 GUID。第三方时间标注为“第三方记录”，不当作 Apple 官方发布日期。

新增验证：npx electron tests/source-ui-check.cjs 检查第三方列表、版本选择、快速切换来源以及说明弹窗。tests/live-source-check.cjs 会查询公开第三方接口。界面测试使用固定数据，未使用个人账户验证实际付费下载。

仓库排除 node_modules、编译产物、IPA、个人账户数据和运行日志。历史版本服务由第三方维护，可用性可能变化。本项目不隶属于 Apple。
