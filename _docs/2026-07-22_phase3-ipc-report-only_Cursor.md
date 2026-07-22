# Phase3 Report-Only IPC / Named Pipe 実装

**日時**: 2026-07-22 22:40 +09:00  
**実装内容**: ADR P3 — Named Pipe + HTTP IPC、anomaly report、command allowlist、T12 merge、ホットスワップ  
**実装AI**: Cursor

## 概要

ADR-2026-07-21 Phase3 をこのリポジトリ側で実装した。Desktop/Backend は report-only、再起動権限は Watchdog のみ。Windows Named Pipe を primary、HTTP loopback を fallback とし、二重 anomaly をマージ（T12）、allowlist 外および Desktop 由来の `command_request` を拒否する。回帰テスト PASS 後 ForceRestart ホットスワップを確認した。

## 背景・要求

- ユーザー: 「Phase3も同様に実装　ホットスワップ」
- ADR: REQ-LM-09 / REQ-LM-10 / T12 / exit criteria「Desktop does not kill/restart backend」

## 前提・判断

- hermes-agent Desktop 本体の改修はこのリポジトリ外。契約は `_docs/IPC-CONTRACT-P3.md` に固定
- Named Pipe 接続は service ロール → `command_request` 実行不可
- `command_request` は admin HTTP + nonce + allowlisted `CommandType` のみ
- `suggest_command` は助言のみで自動実行しない
- 依存: `github.com/Microsoft/go-winio`（Named Pipe）

## 変更対象ファイル

| ファイル | 内容 |
|----------|------|
| `ipc.go` / `ipc_dispatch.go` / `ipc_test.go` | Envelope・AnomalyRegistry・Nonce・dispatch |
| `ipc_pipe_windows.go` / `ipc_pipe_stub.go` | Named Pipe listener |
| `server.go` | `/api/v1/report` `/command` `/ipc` |
| `watchdog.go` / `config.go` / `main.go` | wire・status 露出・pipe 起動 |
| `state_machine.go` | Starting→Backoff 許可 |
| `README.md` / `SECURITY.md` | P3 文書化 |
| `_docs/IPC-CONTRACT-P3.md` | hermes-agent adapter 契約 |
| `go.mod` / `go.sum` | go-winio |

## 実装詳細

1. `anomaly_report` → 記録のみ。同一 code の Desktop+Backend を mergeWindow 内で `merged=true`
2. Desktop/Backend `command_request` → 403/rejected（report-only）
3. Operator `POST /api/v1/command` → allowlist + nonce replay 拒否
4. Named Pipe `\\.\pipe\hermes-watchdog` で同一 JSON プロトコル
5. `/api/status` に `reportOnlyContract` / `ipcPipe` / `recentAnomalies`

## 実行コマンド

```powershell
go test -count=1 -p 1 -timeout 10m .
go build -o dist\hermes-watchdog.exe .
powershell -File scripts\Start-HermesGoWatchdog.ps1 -ForceRestart
Invoke-RestMethod http://127.0.0.1:9920/api/status
# dual anomaly smoke → action=merged
```

## テスト・検証結果

| 検証 | 結果 |
|------|------|
| `go test -count=1 -p 1 .` | **PASS**（0.176s） |
| Build | **PASS** → `dist/hermes-watchdog.exe` |
| Hot-swap ForceRestart | **PASS** pid=33308 |
| `/api/status` | `reportOnlyContract=true`、`soleRestartAuthority=true`、`ipcPipe=\\.\pipe\hermes-watchdog` |
| Log | `named pipe IPC listening on \\.\pipe\hermes-watchdog` |
| Dual `/api/v1/report` | 1st `recorded`、2nd `merged`（T12） |
| Desktop `command_request` via `/api/v1/ipc` | **403 rejected** |

## 残留リスク

- hermes-agent Desktop が未だ独自に serve spawn する場合、契約遵守まで dual authority が残る（本 repo は拒否面を提供済み）
- Named Pipe ACL は Authenticated Users RW（ローカル脅威モデル前提）
- P4 warm-start drain / P5 Job Object は未実装

## 次の推奨アクション

1. 変更を commit / push（ユーザー指示時）
2. hermes-agent 側で IPC-CONTRACT-P3 に沿った report-only adapter 実装
3. Phase4 warm-start checkpoint シーケンス
