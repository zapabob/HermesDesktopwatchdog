# 2026-07-23 GH Pages + Release — Cursor

## 概要

HermesDesktopwatchdog 向けに、アーキテクチャ図の dark navy + neon gold 美学で GitHub Pages サイトを構築し、Windows amd64 バイナリを versioned `.tar.gz` として GitHub Release に公開。Pages からは Release アセット URL を Download CTA としてリンク。

## 背景・要求

- 添付 architecture diagram をデザイン正本とする GH Pages UI/UX
- `hermes-watchdog.exe` を `hermes-watchdog-windows-amd64-v1.0.0.tar.gz` に梱包（README / LICENSE / SHA256SUMS）
- `gh release create` で P1–P6 要約付きリリース
- `docs/` on main で Pages 配信、バイナリ本体は Release 側に置き Pages を軽量維持
- Hermes agent/desktop リポジトリには触れない

## 適用 SOP / Skill

- Implementation Start Gate / MILSPEC Codex Standard
- SOP-Frontend-Design（Pages UI）
- SOP-Go（バイナリビルド）
- Project AGENTS.md（Build script / dist gitignore）

## 前提・判断

- 初回セマンティックタグ: **v1.0.0**（P1–P6 出荷済み）
- Pages source: `/docs` on `main`
- tar.gz は GitHub Release にのみフル配置；`docs/checksums.json` と `docs/downloads/README.md` でハッシュと URL をミラー

## 変更対象ファイル

- `docs/index.html`, `docs/styles.css`, `docs/site.js`
- `docs/assets/*`（architecture / infographic）
- `docs/checksums.json`, `docs/downloads/README.md`
- `README.md`（Pages / Release リンク）
- `.gitignore`（`release-out/`, `release-staging/`）
- `_docs/2026-07-23_ghpages-release_Cursor.md`（本ログ）

## 実装詳細

1. `scripts/Build-HermesGoWatchdog.ps1` で tidy / test / build → `dist/hermes-watchdog.exe`
2. staging に exe + LICENSE + README snippet + SHA256SUMS.txt を配置し `tar -czf` でアーカイブ
3. Orbitron + Exo 2 / `#00050A` / `#FFD700` / grid / glass cards / full-bleed hero で `docs/` サイト
4. Download CTA → Release asset URL；`checksums.json` を fetch して SHA256 表示
5. `gh release create v1.0.0` + Pages enable via API

## 実行コマンド

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\Build-HermesGoWatchdog.ps1
# packaging via PowerShell tar + Get-FileHash
gh release create v1.0.0 ...
gh api ... pages
git push origin main
```

## テスト・検証結果

- `go test ./... -count=1` — **ok**（ビルドスクリプト内）
- `go build` — **ok** → `dist\hermes-watchdog.exe` (~21.7 MB)
- Archive: `hermes-watchdog-windows-amd64-v1.0.0.tar.gz` (~8.5 MB)
- SHA256: `c086a9d526f7234989c88f0137c41696f20a3ad18c6e48ff824dc218588fe448`
- Pages / Release URL はデプロイ後に fetch 検証

## 残留リスク

- GitHub Pages 初回公開は数分遅延しうる
- 大きな PNG/JPG を `docs/assets` に含む（Pages 帯域は許容範囲だが Release 本体は別）
- バージョン固定リンク（v1.0.0）；次回リリース時は HTML / checksums 更新が必要

## 次の推奨アクション

- Pages 公開後にモバイル幅で目視確認
- 次回バージョンでは `docs/site.js` の VERSION 定数を自動化
- Dependabot / CI で release packaging を任意自動化

## Deploy verification (post-push)

- Pages: https://zapabob.github.io/HermesDesktopwatchdog/ — HTTP 200, brand+CTA present, build status `built`
- Release: https://github.com/zapabob/HermesDesktopwatchdog/releases/tag/v1.0.0
- Asset HEAD: HTTP 200, Length=8477214
- SHA256: `c086a9d526f7234989c88f0137c41696f20a3ad18c6e48ff824dc218588fe448`
- Commit: `a86eecd`

