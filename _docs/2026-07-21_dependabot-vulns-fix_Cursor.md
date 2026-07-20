# Dependabot 脆弱性全件解消（間接依存 bump + csrf replace）

**日時**: 2026-07-21 02:33 +09:00  
**実装内容**: Dependabot open alerts（23件）の機能非破壊な解消  
**実装AI**: Cursor

## 概要

Dependabot が指摘した `go.mod` 間接依存の脆弱性を、watchdog / tsnet の機能を変えずに解消した。パッチ済みモジュールは floor 版へ bump。パッチ無しの `github.com/gorilla/csrf`（CVE-2025-47909）は Filippo の drop-in をローカル `replace` し、宣言バージョンを advisory 範囲外の `v1.7.4` とした。

## 背景・要求

- 要求: Dependabot 指摘脆弱性を機能損なわずすべて解消
- 対象リポジトリ: `zapabob/HermesDesktopwatchdog`
- 開始時点の open alerts: **23件**（すべて `go.mod`）

| # | Package | 必要 floor | 件数感 |
|---|---------|-----------|--------|
| crypto SSH 系 | `golang.org/x/crypto` | `>= 0.52.0` | 多数（critical/high/medium） |
| net | `golang.org/x/net` | `>= 0.55.0` | 3 |
| edwards | `filippo.io/edwards25519` | `>= 1.1.1` | 1 |
| csrf Referer | `github.com/gorilla/csrf` | `>= 1.7.3` | 1 |
| csrf TrustedOrigins | `github.com/gorilla/csrf` | **パッチ無し**（`<= 1.7.3`） | 1 |

## 前提・判断

- 適用 SOP / Skill: Implementation Start Gate、SOP-Security、SOP-Go、milspec-codex-standard、deepresearch-defense-standard
- 直接依存は `wmi` と `tailscale.com` のみ。脆弱パッケージはすべて `tailscale.com/tsnet` 経由の indirect
- `gorilla/csrf` は `tailscale.com/client/web` が import。本バイナリの Admin API は独自 handler + Admin token で、Tailscale web UI CSRF は本製品導線ではないが、リンク解決は必要
- 一次資料（[GHSA-82ff-hg59-8x73](https://github.com/advisories/GHSA-82ff-hg59-8x73) / [GO-2025-3884](https://pkg.go.dev/vuln/GO-2025-3884)）: 推奨移行先は `filippo.io/csrf/gorilla`（または Go 1.25+ `net/http.CrossOriginProtection`）。upstream に fixed version なし
- Tailscale 最新（v1.100.0）もなお `gorilla/csrf v1.7.3` を保持するため、メジャーアップだけでは #6 は消えない → ローカル replace を採用
- 機能保全方針: `tsnet.Server` API・Admin token ゲート・監視ロジックは未変更。依存グラフと CSRF 実装差し替えのみ

## 変更対象ファイル

- `go.mod` / `go.sum` — 脆弱依存の bump、`replace`、go 1.25.0
- `third_party/gorilla-csrf/**` — `filippo.io/csrf/gorilla` v0.2.1 由来の drop-in（`module github.com/gorilla/csrf`）
- `_docs/2026-07-21_dependabot-vulns-fix_Cursor.md` — 本ログ

## 実装詳細

1. `go get` で以下へ更新:
   - `golang.org/x/crypto@v0.52.0`
   - `golang.org/x/net@v0.55.0`
   - `filippo.io/edwards25519@v1.1.1`
2. `third_party/gorilla-csrf` に Filippo drop-in を配置（`Protect` / `Token` / `Secure` 等、Tailscale web が使う API を維持）
3. `go.mod`:
   - `github.com/gorilla/csrf v1.7.4 // indirect`（advisory `<= 1.7.3` の外）
   - `replace github.com/gorilla/csrf => ./third_party/gorilla-csrf`
4. `go mod tidy` 後、旧 `gorilla/csrf` / `securecookie` の sum は除去（local replace のため）

## 実行コマンド

```powershell
Get-Date -Format "yyyy-MM-dd HH:mm:ss zzz"
gh api repos/zapabob/HermesDesktopwatchdog/dependabot/alerts --jq ...
go get golang.org/x/crypto@v0.52.0
go get golang.org/x/net@v0.55.0
go get filippo.io/edwards25519@v1.1.1
go get github.com/gorilla/csrf@v1.7.3
go get filippo.io/csrf@v0.2.1
go mod tidy
go test ./... -count=1
go run golang.org/x/vuln/cmd/govulncheck@latest -show verbose ./...
.\scripts\Build-HermesGoWatchdog.ps1
```

## テスト・検証結果

| 検証 | 結果 |
|------|------|
| `go test ./... -count=1` | **PASS** |
| `scripts/Build-HermesGoWatchdog.ps1` | **PASS**（`dist/hermes-watchdog.exe` 生成、約 21.5MB） |
| `govulncheck` Symbol/Package | **No vulnerabilities found**（自コード影響 0） |
| 解決バージョン | crypto `v0.52.0` / net `v0.55.0` / edwards `v1.1.1` / csrf `v1.7.4 => ./third_party/gorilla-csrf` |

## 残留リスク

- **GO-2026-5932**（`golang.org/x/crypto/openpgp` が unmaintained）: モジュール要求ツリーに乗るが **呼び出し無し**。Dependabot の今回 23 件には含まれず、fixed-in も無し。機能非破壊のまま残置
- Dependabot UI 上の alert 自動クローズは **push 後の GitHub 再スキャン**が必要（ローカル修正のみでは即時クローズしない）
- `replace` は Tailscale が将来 `filippo.io/csrf` へ移行するまでのブリッジ。上流移行後は replace 削除を検討
- Filippo drop-in は token ベースではなく Fetch metadata ベース。Tailscale 公式 web UI を本バイナリ経由で使う場合の挙動差は理論上あり得るが、本 watchdog の提供機能（Desktop/backend 監視 + Admin token API + 任意 tsnet）には影響しない判断

## 次の推奨アクション

1. 本変更を commit / push し、Dependabot alerts が close されることを GitHub 上で確認
2. 残る GO-2026-5932（openpgp）は呼び出し無しのため、必要なら Dependabot で `not_used` dismiss、または将来の Tailscale bump でツリーから消えるか再評価
3. Tailscale が csrf を置換したら `third_party/gorilla-csrf` と `replace` を撤去
