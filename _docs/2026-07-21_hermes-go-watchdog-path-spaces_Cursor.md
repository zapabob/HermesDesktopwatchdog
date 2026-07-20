# 2026-07-21 Hermes Go Watchdog path-spaces sync (Cursor)

## 概要
hermes-agent 側の最新 Go watchdog 修正を standalone リポジトリ zapabob/HermesDesktopwatchdog の main に同期・push した。

## 背景・要求
Windows のパス空白（例: `New project`）で `-hermes-root` が切れる問題の修正を公開リポジトリへ反映する。

## 前提・判断
- 既存 clone: `C:\Users\downl\Documents\New project\HermesDesktopwatchdog`
- レイアウトは flat Go ルート + `scripts/` を維持
- `*.exe` / `dist/` は .gitignore のためコミットしない
- Start スクリプトは standalone パス（`WatchdogRoot` / sibling hermes-agent）を保ちつつ argv 分離を移植

## 変更対象ファイル
- main.go, backend.go, process_windows.go, go.mod, README.md
- scripts/Start-HermesGoWatchdog.ps1, scripts/Build-HermesGoWatchdog.ps1

## 実装詳細
- sanitizePathFlag / detectRepoRoot（4レベル pyproject 探索）
- resolveServeWorkDir 検証
- Start-Process ArgumentList で `-hermes-root`, `` を別 argv

## 実行コマンド
- git add / commit / `git push origin main`

## テスト・検証結果
- ローカル差分確認とシンボル存在確認のみ（go test は本ターン未実行）
- remote main SHA: `3efbbfc2cc056399f3f0ab36123fa07a91cf0b46`

## 残留リスク
- GitHub Dependabot が 23 vulnerabilities を報告（既存依存）
- go test / 実機起動スモークは未実施

## 次の推奨アクション
- `scripts\Build-HermesGoWatchdog.ps1` でビルド＋ `go test ./...`
- Dependabot の critical 依存を別 PR で更新検討
