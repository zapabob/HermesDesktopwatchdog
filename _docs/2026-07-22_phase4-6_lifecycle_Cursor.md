# 2026-07-22 Phase4–6 Lifecycle (P4 Warm-Start / P5 Job Object / P6 Update Suppress) — Cursor

## 概要

ADR-2026-07-21 の P4–P6 を **Watchdog リポジトリ単体**で実装した。Hermes-agent / Desktop には未着手。Hermes 協力が必要な箇所は契約ドキュメントと status/event シグナルに留め、スタブで正直にギャップを残した。

## 背景・要求

- REQ-LM-04 warm start（interrupted ≠ success）
- REQ-LM-07 renderer vs full Desktop 粒度
- REQ-LM-08 Job Object
- T13 update suppress
- Hermes 非改変制約

## 前提・判断

- `CommandWarmRestart` は Desktop tree-kill ではなく backend warm-start sequencer を実行
- Desktop last-resort は `stop_desktop` + `start_backend` + `start_desktop`
- Checkpoint ack / session restore / renderer recreate は Hermes 側将来 PR
- Job Object 失敗時は従来の `taskkill /T` にフォールバック

## 変更対象ファイル

| File | Role |
|------|------|
| `warm_start.go` / `warm_start_test.go` | P4 sequencer + fake clock tests |
| `job_object.go` / `job_object_windows.go` / `job_object_stub.go` / `job_object_test.go` | P5 Job Object |
| `recovery_policy.go` | P5 renderer-only policy |
| `update_suppress.go` / `update_suppress_test.go` | P6 gate |
| `backend.go` | Job assign on start; job terminate on stop |
| `commands.go` / `watchdog.go` / `server.go` / `main.go` / `config.go` / `heartbeat.go` / `events.go` / `ipc_dispatch.go` | Wiring |
| `_docs/WARM-START-CONTRACT.md` | Hermes 向け契約 |
| `_docs/ARCHITECTURE.md` / `README.md` / `_docs/OPERATOR.md` | ロードマップ・運用 |

## 実装詳細

### P4
- 12-step intent を Watchdog 側で実行（drain timeout / checkpoint wait / stop→start→ready→manifest）
- `active_runs` 残りで **interrupted**（success 禁止）
- `/api/status` → `warmStart`

### P5
- Managed backend を Job Object に Assign; stop は `TerminateJobObject` 優先
- `renderer_oom` / `renderer_crash` 等は full Desktop restart をスキップ＋イベント
- port-hold ヒントを status `jobObject.detail` に記載

### P6
- env `HERMES_WATCHDOG_UPDATE_IN_PROGRESS=1`
- file `%LOCALAPPDATA%\HermesWatchdog\update.lock`
- admin `POST /api/v1/update-suppress`（TTL 可）
- RunCycle で auto Start*/WarmRestart を抑止; heartbeat/report は継続

## 実行コマンド

```powershell
go test -count=1 -p 1 -timeout 10m .
powershell -File scripts\Build-HermesGoWatchdog.ps1
powershell -File scripts\Start-HermesGoWatchdog.ps1 -ForceRestart
Invoke-RestMethod http://127.0.0.1:9920/api/status
```

## テスト・検証結果

| Check | Result |
|-------|--------|
| `go test -count=1 -p 1 -timeout 10m .` | **PASS** (~2.8–3.2s) |
| Build `dist/hermes-watchdog.exe` | **PASS** (pid hot-swap 7000) |
| `/api/status` warmStart / updateSuppress / jobObject / recovery | **PASS** fields present |
| ForceRestart | **PASS** watchdogPid=7000 |

## Hermes-dependent residual gaps

1. Drain / stop-accepting の実効（Hermes が status を見て止まる必要あり）
2. Checkpoint ack チャネル未実装
3. Session routing restore は signal のみ
4. Renderer recreate IPC なし（skip + log のみ）
5. Installer が `update.lock` / env を書くのはオペレータ／インストーラ側

## 残留リスク

- Job Object 作成/Assign 失敗時は taskkill フォールバック（孫プロセス取りこぼしの余地）
- Warm-start 中の並行 `warm_restart` は拒否（Active gate）
- Update suppress の env はプロセス環境依存（再起動で消える）

## 次の推奨アクション

- hermes-agent 側: heartbeat `active_runs` + checkpoint ack + drain 連携
- Desktop: renderer-only recreate IPC
- Installer: update window で `update.lock` 作成/削除
