# Hermes Go Watchdog Lifecycle Manager 設計ADR作成

**日時**: 2026-07-21 02:52 +09:00  
**実装内容**: Lifecycle Manager 設計ADR（実装コードなし）  
**実装AI**: Cursor

## 概要

計画「Hermes Go Watchdog — Lifecycle Manager 設計ADR計画」に従い、現状コードとのギャップ、非対称再起動権限、ServiceState、heartbeat、warm-start、RestartPolicy、段階PR、15試験トレース、Phase1ファイル一覧を `_docs/ADR-2026-07-21_hermes-watchdog-lifecycle-manager.md` に固定した。Go 実装は意図的に行っていない。

## 背景・要求

- 設計レビュー（再起動 storm / split-brain / Job Object / warm-start）を PR 化前提の ADR に落とす
- 実装は承認後 Phase1 から

## 前提・判断

- 適用 SOP: Implementation Start Gate / Security / Go / milspec / deepresearch-defense
- 本リポジトリは operator-only を維持
- hermes-agent 変更は契約先行・別 PR
- IPC 第一段は HTTP loopback、Named Pipe は P3

## 変更対象ファイル

- `_docs/ADR-2026-07-21_hermes-watchdog-lifecycle-manager.md`（新規）
- `_docs/2026-07-21_lifecycle-manager-adr_Cursor.md`（本ログ）

## 実装詳細

ADR に以下を記載:

1. 決定事項（非対称権限、ServiceState、heartbeat、warm-start、RestartPolicy、health、Windows、security、events）
2. 現状 vs 目標 mermaid 図
3. `watchdog.go` / `process_windows.go` / `backend.go` / `server.go` ギャップ表
4. P1–P6 ロードマップ
5. T01–T15 要求トレース
6. Phase1 変更ファイル一覧（未実装）

## 実行コマンド

```powershell
Get-Date -Format "yyyy-MM-dd HH:mm:ss zzz"
# ADR markdown write only — no go test / build required for design-only deliverable
```

## テスト・検証結果

| 項目 | 結果 |
|------|------|
| ADR ファイル存在 | Yes |
| 計画の成果物要件カバー | Yes（決定・状態機械・契約・warm-start・RestartPolicy・段階PR・15試験・現状参照・P1一覧） |
| Go コード変更 | None（計画どおり） |

## 残留リスク

- ADR は設計合意まで。P1 実装前は dual authority（Desktop の serve spawn）が残る
- hermes-agent 側未着手のため、非対称権限は規範のみ

## 次の推奨アクション

1. ADR レビュー（restart race / split-brain / process tree / warm-start 観点）
2. 承認後に Phase1（`ServiceState` + `RestartPolicy` + イベントログ）実装タスクを起動
