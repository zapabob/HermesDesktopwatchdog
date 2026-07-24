# 2026-07-24 status-deadlock-fix (Composer)

## 概要
HermesDesktopwatchdog の最新ビルド + ForceRestart 相当のホットスワップを実施。`/api/status` がハングする問題を修正し、正しい `dist\hermes-watchdog.exe` で稼働確認した。

## 背景・要求
- `go test` → build → ForceRestart → `/api/status` 検証
- 途中で C: ENOSPC によりテスト/シェルが破綻

## 前提・判断
- `Start-HermesGoWatchdog.ps1` の `Get-CimInstance` がこの環境で固まりやすく、hermes-agent 配下の古い exe が起動していた
- `/health` は応答するが `/api/status` がタイムアウト → `EnsureHealthy` が `bm.mu` を握ったまま readiness wait（最大数分）し、`State()` → `JobSnapshot()` が同じ mutex でブロック

## 変更対象
- `backend.go` — readiness wait 中は `bm.mu` を解放；auth/probe をロック外へ
- `watchdog.go` — `State`/`saveState` で JobSnapshot・health probe を `w.mu` 外へ

## 検証
- `go build -o dist\hermes-watchdog.exe .` → exit 0
- 起動 PID **41452** Path=`...\HermesDesktopwatchdog\dist\hermes-watchdog.exe`
- `/health` → `{status:ok}`
- `/api/status` → `watchdogPid=41452`, `soleRestartAuthority=true`, `reportOnlyContract=true`, desktop/backend health 付き
- ログ: `local HTTP listening on 127.0.0.1:9920` / named pipe IPC
- 初回 `go test` はディスク不足で `TEST_EXIT=-1`（再実行は別途）

## 残留リスク
- C: 空き容量が逼迫すると再びビルド/テストが失敗する
- Start スクリプトの WMI(`Get-CimInstance`) ハングは未修正（直接 Start-Process で回避）
- backend auth drift on 9118 は環境側の既存問題

## 次の推奨
- Start スクリプトから WMI 列挙を外す/タイムアウト化
- `go test -count=1 -p 1 -timeout 10m .` を空き容量確保後に再実行
