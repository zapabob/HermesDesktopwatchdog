# Phase2 Health Protocol / Heartbeat Ingest 実装

**日時**: 2026-07-22 22:25 +09:00  
**実装内容**: ADR P2 — `/live` `/ready` 消費・deep health cache・heartbeat ingest・leases・ホットスワップ  
**実装AI**: Cursor

## 概要

ADR-2026-07-21 の Phase2 を実装した。バックエンド健全性を Live/Ready/Deep に分離し、legacy `/api/status` を Ready フォールバックとして維持。Watchdog 所有の epoch/instance_id heartbeat を `POST /api/v1/heartbeat` で取り込み、stale epoch / instance mismatch を拒否する。回帰テスト PASS 後に ForceRestart ホットスワップを確認した。

## 背景・要求

- ユーザー: 「P2も同様に実装」（P1 と同フロー: 実装 → 回帰 → ホットスワップ → `_docs`）
- ADR REQ-LM-06 / T03 / T10 / T11 / T15

## 前提・判断

- Ready ≠ Live-only。Live のみは `StateDegraded`（再起動ストームしない）
- Hermes serve が `/live` `/ready` 未実装でも `/api/status` OK なら `status-fallback` で Ready
- Heartbeat auth: `HERMES_WATCHDOG_HEARTBEAT_TOKEN` → admin token →（両方空なら）loopback のみ
- HTTP 制御面を Prewarm より先に起動（長時間 serve 待ちでも `/api/status` を観測可能）
- `waitForReadyPort` は `/ready`→`/api/status` の短タイムアウト `quickBackendReady` を使用

## 変更対象ファイル

| ファイル | 内容 |
|----------|------|
| `health.go` / `health_test.go` | probe `/live` `/ready` `/health/deep` + status-fallback |
| `heartbeat.go` / `heartbeat_test.go` | HeartbeatRegistry / epoch bump / ingest |
| `watchdog.go` | leases・backendReady/Live・Degraded 経路 |
| `commands.go` | 再起動コマンドで epoch bump |
| `server.go` | `POST /api/v1/heartbeat` |
| `config.go` / `main.go` | timeout / deep interval flags・HTTP-first 起動 |
| `backend.go` / `process_windows.go` | Ready 判定統合・quick ready |
| `state_machine.go` | Ready→Stopped / Unresponsive→Starting |
| `README.md` / `.env.example` | P2 ドキュメント |

## 実装詳細

1. `probeBackendHealth`: Ready 優先、Live-only は Ready=false、legacy status は両 true
2. RunCycle: Live && !Ready → Degraded（再起動しない）
3. `HeartbeatRegistry`: protocol_version=1、epoch 一致必須、instance_id 不一致拒否、timeout で fresh 失効
4. `/api/status` に `leases` と backend `health.source` を露出
5. allowlisted restart 時に該当サービス epoch を bump（旧 heartbeat 無効化）

## 実行コマンド

```powershell
go test -count=1 -p 1 -timeout 10m .
powershell -File scripts\Build-HermesGoWatchdog.ps1
powershell -File scripts\Start-HermesGoWatchdog.ps1 -ForceRestart
Invoke-RestMethod http://127.0.0.1:9920/api/status
# heartbeat smoke（loopback・token 未設定時）
```

## テスト・検証結果

| 検証 | 結果 |
|------|------|
| `go test -count=1 -p 1 .` | **PASS**（0.347s〜2.1s） |
| Build → `dist/hermes-watchdog.exe` | **PASS** |
| Hot-swap ForceRestart | **PASS** pid=53292、HTTP 先行起動確認 |
| `/api/status` 安定後 | desktop/backend=`ready`、health=`status-fallback` live+ready、result=`up:9118`、epoch=1（再起動ストーム無し） |
| `POST /api/v1/heartbeat` | **accepted=true**（instanceId=`p2-smoke`、fresh） |

補足修正: `currentHealthy` が reuse 時 pid=0 で常に nil → EnsureHealthy 再 spawn していた問題を修正。Ready 済みなら EnsureHealthy をスキップ。

## 残留リスク

- Hermes 側が `/live` `/ready` を実装するまで status-fallback 依存
- Deep health はグローバル cache（単一プロセス前提）
- Desktop 側 dual authority（P3 まで残存）
- Prewarm 中は RunLoop 未開始のため state が `starting` のまま見える（HTTP は先行で応答）

## 次の推奨アクション

1. 変更を commit / push（ユーザー指示時）
2. Phase3: Desktop/Backend report-only IPC adapter
3. Hermes serve に `/live` `/ready` を追加し status-fallback を段階的に縮小
