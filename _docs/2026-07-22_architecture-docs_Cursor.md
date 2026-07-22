# Architecture documentation (P1–P3 accurate)

**日時**: 2026-07-22 22:49 +09:00  
**実装内容**: `_docs/ARCHITECTURE.md` / `OPERATOR.md` 追加、README からアーキテクチャへの1リンク  
**実装AI**: Cursor

## 概要

オペレータ／開発者が Hermes 本体を触らずに Go Watchdog の責務・構成・シーケンス・セキュリティ・フェーズ状態を把握できる包括的 Markdown を `_docs/` に追加した。未実装機能（P4–P6）は ADR 準拠の planned のみ。Hermes 系リポジトリには一切手を入れていない。

## 背景・要求

- Goal: Architecture doc（システム／コンポーネント／シーケンス／状態機械／パス／セキュリティ／P1–P6）
- 任意: 短い OPERATOR 文書、README は 30 秒スキャン維持で deep link のみ
- 既存 ADR / IPC-CONTRACT-P3 / phase logs を参照し二重記述を避ける
- コード突合で正確性を担保

## 前提・判断 / 適用 SOP・Skill

- SOP: Implementation Start Gate、Go、Security（文書化のみ）、milspec-codex-standard
- Hermes 改修禁止（本 repo のみ）
- ADR の Gap Analysis 節は P1 前のスナップショットのため、現状は phase logs + 現行コードを正とする（ADR Decision / Roadmap はリンク）
- P3「Done」は **このリポジトリ側の IPC 面**；hermes-agent adapter 実装は別途

## 変更対象ファイル

| ファイル | 内容 |
|----------|------|
| `_docs/ARCHITECTURE.md` | **新規** アーキテクチャ正本 + mermaid 図 7 本 |
| `_docs/OPERATOR.md` | **新規** 短い運用ランブック |
| `README.md` | ARCHITECTURE への1行リンク + OPERATOR 行 |
| `_docs/2026-07-22_architecture-docs_Cursor.md` | 本実装ログ |

## 実装詳細

1. Purpose / non-goals、システムコンテキスト、Go ファイルコンポーネント図
2. `RunCycle` / heartbeat-epoch / T12 merge / Pipe vs HTTP のシーケンス
3. `ServiceState` 状態図（コードの遷移表に合わせ Starting→Backoff 等を含む）
4. データパス・HTTP 面・セキュリティ要約・P1–P6 ステータス表
5. ADR / IPC / phase logs へのリンク集約

## 実行コマンド

```powershell
Get-Date -Format "yyyy-MM-dd HH:mm:ss K"
# コード突合: main.go, watchdog.go, state_machine.go, health.go, heartbeat.go,
# ipc*.go, server.go, config.go, commands.go, process_windows.go, README, SECURITY, ADR, IPC-CONTRACT
```

## テスト・検証結果

| 検証 | 結果 |
|------|------|
| 現行コードとの突合（状態機械・RunCycle・IPC・パス・ポート） | **実施** — 文書は現行実装に整合 |
| 未実装機能の捏造なし | P4–P6 は Planned のみ |
| Hermes リポジトリ変更 | **なし** |
| `go test` | ドキュメントのみのため未実行（挙動変更なし） |

## 残留リスク

- hermes-agent が契約前に独自 spawn する場合、dual authority は残る（本 repo は拒否面のみ提供済み）
- ADR Gap Analysis 節を単独で読むと古いギャップ記述に見える → ARCHITECTURE を現状の入口にする
- Named Pipe ACL / ローカル脅威モデルは SECURITY 依存

## 次の推奨アクション

1. docs + README を main に commit / push（Goal 完了のため）
2. hermes-agent 側で IPC-CONTRACT-P3 adapter
3. P4 warm-start 設計着手時は ARCHITECTURE §9 を更新

## Commit / push

| Item | Value |
|------|-------|
| Commit | `a1be5e7510dfc40f8bd1ab5fe5a227a8dfee6076` |
| Message | `docs: add architecture and operator guides for P1-P3` |
| Branch | `main` → `origin/main` |

## Return checklist

- Paths: `_docs/ARCHITECTURE.md`, `_docs/OPERATOR.md`, README link, this log
- Diagrams: 7 mermaid（context, components, state, RunCycle, heartbeat, T12, Pipe/HTTP）
