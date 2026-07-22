# 2026-07-22 README rewrite + P1–P3 commit/push (Cursor)

## 概要

Phase1–Phase3（状態機械・health/heartbeat・report-only IPC）の実装一式を `main` にコミット＆プッシュし、README を約30秒で把握できるスキャナブル版に書き換え。

## 背景・要求

- ユーザー明示: 関連作業をメインへコミットプッシュ
- README を長文から「30秒スキャン」向けに再構成
- 秘密情報・`dist/*.exe`・不要な `.cursor` 状態はコミットしない

## 前提・判断

- ブランチ: `main`（`origin/main` 追従）
- 日時: 2026-07-22 22:44 JST（ローカル `Get-Date`）
- `dist/` と `.env` は `.gitignore` 済み（`git check-ignore` で確認）
- `.cursor/hooks/state/continual-learning.json` は作業状態のため除外
- README は英語のまま（既存リポジトリ文体に合わせ、速度優先で簡略化）

## 変更対象ファイル

### 追加（主なソース）

- `state_machine.go` / `state_machine_test.go`
- `restart_policy.go` / `restart_policy_test.go`
- `health.go` / `health_test.go`
- `heartbeat.go` / `heartbeat_test.go`
- `commands.go` / `events.go`
- `ipc.go` / `ipc_dispatch.go` / `ipc_pipe_windows.go` / `ipc_pipe_stub.go` / `ipc_test.go`
- `lifecycle_test.go`
- `_docs/IPC-CONTRACT-P3.md`
- `_docs/2026-07-22_phase{1,2,3}_*_Cursor.md`
- `_docs/2026-07-22_readme-rewrite-commit-push_Cursor.md`

### 更新

- `README.md`（30秒スキャン向け全面書き換え）
- `SECURITY.md`, `.env.example`, `config.go`, `main.go`, `watchdog.go`, `server.go`, `backend.go`, `process_windows.go`, `scripts/Start-HermesGoWatchdog.ps1`, `go.mod`, `go.sum`

### コミット除外

- `dist/hermes-watchdog.exe`（gitignore）
- `.env`（不在 / gitignore）
- `.cursor/`（continual-learning 状態）

## 実装詳細（README）

削除: 長尺 Feature 列挙、巨大 CLI/API 表、マーケティング調の重複説明。  
残置: What / What-not / Build+Start / P1–P3 箇条書き / Safety 一行 / 深掘りリンク。

## 実行コマンド

```powershell
git status
git diff --stat
git log -5 --oneline
git add <relevant paths>
git commit -m "..."
git push origin main
git status
```

## テスト・検証結果

本作業はドキュメント整理＋git 配信。コード検証は既存 `go test` に依存（本ログ時点では push 前に別途実行していない場合あり）。

## 残留リスク

- README から詳細 API/CLI 表を外したため、オペレータは `_docs` / フラグ `-h` を参照する必要がある
- リモート push 後の CI 結果は別途確認が必要

## 次の推奨アクション

- CI / Dependabot の成否確認
- 必要なら README に最小 API 一覧を別セクションで復帰
