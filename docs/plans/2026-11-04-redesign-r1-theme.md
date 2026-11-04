# R1 可选主题实施计划

采用 CLAWSTA-134 于 2026-11-04 更新的独立主题方案。

1. 从 main 012edaa3c 建立 feat/redesign-r1-theme，保留上游原文件。
2. 新建共享主题，覆盖浅/深色语义变量和已有阅读类的字号/行高；Web、Desktop 各增一行 import。
3. 新增独立对比度测试，核对透明背景合成、正文、链接、按钮和状态色；不修改现有测试。
4. 验证上游差异严格为两行 import，实际关闭主题并核对 main 外观，再恢复。
5. 执行 typecheck、lint、test 与四组 E2E，采集 24 张真实页面截图。
6. 提交 PR，列出完整变量/比值、暂留硬编码颜色及验证结果；不合并、不发版。

## R1-F1 verification update

Rebase the optional theme onto main `27351108c` (v0.2.8). Change only the light warning to the approved `oklch(0.55 0.15 62)`; preserve the dark warning and all upstream files except the two imports.

1. Recalculate actual painted contrast and run the independent theme guard. Document the approved warning hue exception and the selected-surface reason for muted lightness 0.51.
2. Rebuild the isolated API from the new main source and reuse the existing visual dataset. Capture all six pages in light/dark for both main and R1, at 1440×900.
3. Run the unchanged navigation/issues/comments/settings specs and desktop tests with and without the theme, using identical commands, environment and database resets. Compare failures by test identity and error location.
4. Run typecheck, lint, the complete frontend tests and Web production builds. Preserve existing tests; resolve any theme-only failure within the theme layer.
5. Update PR #35, source/diff proofs, contrast tables and screenshot ZIP; report the new base/head and the matched baseline failures. Do not merge, tag or release.
