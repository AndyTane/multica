## 本轮发布范围

F1/F2 只覆盖 Web、Desktop 及它们使用的共享包。**本轮不发布移动端，不生成或交付 IPA、APK、AAB，也不修改 `apps/mobile` 代码。** 已阅读 `apps/mobile/AGENTS.md`。

移动端是独立的 Expo / React Native 客户端，拥有自己的 UI、认证、API、存储、国际化和发布流程；Web/Desktop 的验证不能证明其品牌和外联行为已同步完成。

## 构建与发布入口核对

- 根 `package.json` 的 `build`、`typecheck`、`lint`、`test` 都带 `--filter=!@multica/mobile`。根 `build` 的 Turbo dry-run 确认没有 `@multica/mobile#build` 任务。
- `dev:mobile`、`ios:mobile*` 是显式的移动端开发/原生构建命令，本轮不执行。
- `.github/workflows/release.yml` 发布 Go CLI、后端/Web 容器、Helm chart 及 Linux/Windows Desktop，不调用 Expo、EAS 或移动端打包脚本。
- `.github/workflows/mobile-verify.yml` 只执行移动端 typecheck/lint/test，不构建或上传原生安装包。
- Desktop 的 `package` / `package:all` 只进入 Electron 构建流程。本轮保留 Draft PR，不创建 release/tag，也不执行任何发布流程。

## 移动端发布前必须完成

1. 将应用名、图标、五种语言、系统通知与安装身份统一为 U Workers；独立 bundle/package ID、数据目录及存储命名空间，不迁移原 Multica 数据。
2. 清除官方 API、Web、WebSocket 和云服务回退；缺配置显示本地错误，保留用户配置的服务及认证。loopback 也必须认证。
3. 移除官方云、营销、计费、升级和官方更新入口；旧深链落到可返回工作区的本地不可用界面。
4. 关闭非用户触发的分析、错误上传、遥测、版本/设备上报与第三方资源加载。保留用户授权的模型、OAuth、MCP、Git、集成与下载。
5. 检查移动端独立 API/认证实现与共享纯类型的兼容性，执行其 AGENTS 中的三项验证，在真机或模拟器复核主题、键盘、滚动、导航、权限、通知与网络请求。

## 未验证项

本轮未执行移动端单元测试、typecheck、lint、Expo/EAS/iOS/Android 构建、真机或模拟器渲染、移动端抓包、推送及跨端实时一致性验证。本轮 Web/Desktop 的测试和截图不能用于移动端发布验收。
