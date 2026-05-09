# CLAUDE.md

## プロジェクト概要

GitHub Issue を起点に AI Agent が自律的にコード実装・PR 作成・CodeRabbit フィードバック対応を行う Go アプリケーション。

## ディレクトリ構成

```
agent/
  main.go          # ポーリングループ（RPOP → dispatch）
  orchestrator.go  # AgentSession ライフサイクル管理
  llm.go           # Anthropic API クライアント
  sandbox.go       # agent-sandbox ラッパー
  github.go        # GitHub API (go-github)
  redis.go         # Upstash Redis HTTP クライアント
k8s/               # kind / Kubernetes マニフェスト
```

## アーキテクチャの要点

- **ポーリング型**: インバウンド通信不要。Redis キューを 30 秒間隔で RPOP。
- **Redis 二役**: `agent-queue`（List）でタスク受信、`session:{issueNumber}`（JSON/TTL 24h）でセッション管理。
- **sandbox 操作の制約**: `sb.Write` はファイル名のみ受け付ける。サブディレクトリへの書き込みは `sb.Run("mkdir -p dir")` → `sb.Write("file", data)` → `sb.Run("mv file dir/file")` の 3 ステップ。

## AgentSession ステート

```
developing → review_pending → fixing → done
                                └──── aborted  (iteration >= MAX_ITERATION)
```

## 環境変数

| 変数 | 説明 |
|---|---|
| `ANTHROPIC_API_KEY` | Anthropic API キー |
| `UPSTASH_REDIS_URL` | Upstash Redis エンドポイント |
| `UPSTASH_REDIS_TOKEN` | Upstash 認証トークン |
| `GITHUB_TOKEN` | GitHub PAT (repo + pull_request スコープ) |
| `POLL_INTERVAL_SEC` | ポーリング間隔（秒、デフォルト: 30） |
| `MAX_ITERATION` | 最大修正回数（デフォルト: 5） |

## 開発コマンド

```bash
go build ./...          # ビルド確認
go test ./...           # テスト実行
make cluster            # kind クラスタ作成
make deploy             # Agent Pod デプロイ
make logs               # ログ確認
```

## コーディング規約

- エラーは `fmt.Errorf("context: %w", err)` でラップして返す。握り潰さない。
- Redis 操作は Upstash REST API（HTTP）経由。SDK は使わない。
- LLM へのプロンプトは `llm.go` に集約する。他ファイルから直接 Anthropic SDK を呼ばない。
- sandbox への操作は `sandbox.go` のラッパー経由に統一する。

## 依存ライブラリ

- `github.com/anthropics/anthropic-sdk-go` — Anthropic API
- `github.com/google/go-github/v72` — GitHub API
- `golang.org/x/oauth2` — GitHub 認証
- `sigs.k8s.io/agent-sandbox` — sandbox クライアント
