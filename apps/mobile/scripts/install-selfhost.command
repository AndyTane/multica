#!/usr/bin/env bash
# Build the configured self-hosted release using the existing iOS wrapper.
set -euo pipefail

finish() {
  result=$?
  if [ "$result" -ne 0 ]; then
    printf '\n安装未完成，请保留上面的错误信息。\n'
  fi
  if [ -t 0 ]; then read -r -p '按回车关闭窗口。' _; fi
  exit "$result"
}
trap finish EXIT

if [ "$(uname -s)" != Darwin ]; then
  printf '请把这个构建包复制到 Mac，然后在 Mac 上运行。\n' >&2
  exit 1
fi
cd "$(dirname "$0")/../../.."
if ! command -v node >/dev/null || ! command -v npm >/dev/null; then
  printf '请先安装 Node.js 24 LTS：https://nodejs.org/en/download\n' >&2
  exit 1
fi
node -e 'if (Number(process.versions.node.split(".")[0]) < 22) { console.error("需要 Node.js 22 或更新版本。"); process.exit(1); }'
if ! xcodebuild -version || ! xcrun --sdk iphoneos --show-sdk-path; then
  printf '请先安装并打开完整的 Xcode，完成首次设置，并在 Settings → Accounts 添加 Apple ID。\n' >&2
  exit 1
fi
if [ ! -f apps/mobile/.env.production.local ]; then
  cp apps/mobile/selfhost-windows.env.example apps/mobile/.env.production.local
  printf '已从 Windows 局域网配置模板创建 .env.production.local。\n'
fi

printf '\n请用 USB 连接 iPhone，解锁手机并信任这台 Mac，开启“开发者模式”。\n'
printf '首次编译会下载依赖并编译 React Native，耗时较长。\n\n'
npm exec --yes --package=pnpm@10.28.2 -- pnpm --filter @multica/mobile... install --frozen-lockfile
npm exec --yes --package=pnpm@10.28.2 -- pnpm ios:mobile:device:prod:release
printf '\n已完成编译和安装。免费 Apple ID 的签名通常有效 7 天，之后重新运行即可。\n'
