# Phase1 ServiceState / RestartPolicy / sole authority 実装

**日時**: 2026-07-22 22:05 +09:00  
**実装内容**: ADR P1 — ServiceState・RestartPolicy・allowlisted Command・イベント・RunCycle 改修・ホットスワップ  
**実装AI**: Cursor

## 概要

ADR-2026-07-21 の Phase1 を実装した。Watchdog を sole restart authority として明示し、`ServiceState` 状態機械・指数 backoff / Failed ラッチ・機械可読イベント・Desktop 最終手段再起動を入れた。回帰テスト PASS 後に `dist/hermes-watchdog.exe` を ForceRestart でホットスワップした。

## 背景・要求

- 計画: Lifecycle Manager ADR → P1 実装
- ユーザー追加要求: 回帰テスト + ホットスワップ

## 前提・判断

- P1 non-goals（Named Pipe / Job Object / warm-start drain / deep health）は未実装
- `cycleResult` 文字列は wire 互換維持
- Failed 解除は `POST /api/v1/resume`
- Start スクリプトはスペース付き `hermes-root` を `-name="value"` で渡すよう修正

## 変更対象ファイル

| ファイル | 内容 |
|----------|------|
| `state_machine.go` / `_test.go` | ServiceState + 遷移表 |
| `restart_policy.go` / `_test.go` | RestartPolicy / Tracker（T09） |
| `events.go` | JSONL restart/command events |
| `commands.go` | CommandType allowlist |
| `watchdog.go` | 遷移駆動 RunCycle・sole authority |
| `config.go` / `main.go` | RestartPolicy flags / EventsPath |
| `lifecycle_test.go` | status / command 検証 |
| `README.md` | P1 反映 |
| `scripts/Start-HermesGoWatchdog.ps1` | スペース付き path の引数修正 |

## 実装詳細

1. Desktop DOWN → `CommandStartDesktop` のみ（sole authority）
2. Backend DOWN → `CommandStartBackend` + backoff；窓内上限で `Failed`
3. Desktop 再起動は backend 救済が `fail-threshold` 回失敗した最終手段（`CommandWarmRestart`）
4. `/api/status` に `desktopService` / `backendService` / `restart` / `soleRestartAuthority` をライブ反映

## 実行コマンド

```powershell
go test -count=1 -p 1 -timeout 10m .
go build -o dist\hermes-watchdog.exe .
powershell -File scripts\Start-HermesGoWatchdog.ps1 -ForceRestart
Invoke-RestMethod http://127.0.0.1:9920/api/status
```

## テスト・検証結果

| 検証 | 結果 |
|------|------|
| `go test -count=1 -p 1 .` | **PASS**（0.329s） |
| Build script / `go build` | **PASS** → `dist/hermes-watchdog.exe` |
| Hot-swap ForceRestart | **PASS** pid=45136 |
| `/api/status` | `soleRestartAuthority=true`, desktop/backend=`ready`, `maxRestarts=5` |
| lock `repoRoot` | `...\New project\hermes-agent`（スペース維持） |

## 残留リスク

- P2 以降（heartbeat / deep health / Job Object / warm-start）未実装
- Desktop 側の独自 serve spawn は hermes-agent P3 まで dual authority が残存しうる
- 初回 `go test ./...` は Tailscale 依存のコンパイルで OOM しうる → `-p 1` / `GOMAXPROCS=2` 推奨

## 次の推奨アクション

1. 変更を commit / push
2. Phase2（`/live` `/ready` + heartbeat ingest）設計・実装
3. hermes-agent 側 report-only adapter（P3）
