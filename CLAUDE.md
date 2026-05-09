# CLAUDE.md

## プロジェクト概要

GitHub Issue を起点に AI Agent が自律的にコードを実装・PR 作成・CodeRabbit レビュー対応を行う Go アプリケーション。

## ディレクトリ構成

```
agent/          # Orchestrator 本体 (Go)
  main.go       # ポーリングループ
  orchestrator.go
  llm.go        # Anthropic API
  sandbox.go    # agent-sandbox ラッパー
  github.go     # GitHub API
  redis.go      # Upstash Redis
k8s/            # Kubernetes マニフェスト
Dockerfile
Makefile
```

## 開発ルール

- `go 1.24` を使用
- モジュール名: `github.com/khirotaka/omakase`
- エラーハンドリングは呼び出し元に委ねる（`fmt.Errorf("...: %w", err)`）
- ログは `log/slog` を使用

## 主要な外部依存

| パッケージ | 用途 |
| --- | --- |
| `anthropics/anthropic-sdk-go` | LLM 呼び出し |
| `google/go-github/v67` | GitHub API |
| `golang.org/x/oauth2` | GitHub 認証 |

Upstash Redis は HTTP REST API を直接呼び出す（専用 SDK は使わない）。

## 環境変数

```
ANTHROPIC_API_KEY       Anthropic API キー
UPSTASH_REDIS_URL       Redis エンドポイント
UPSTASH_REDIS_TOKEN     Redis 認証トークン
GITHUB_TOKEN            GitHub PAT
POLL_INTERVAL_SEC       ポーリング間隔（デフォルト: 30）
MAX_ITERATION           最大修正回数（デフォルト: 5）
```

## AgentSession ステート遷移

```
developing → review_pending → fixing → done
                                └──────→ aborted  (iteration 上限)
```

## agent-sandbox の注意点

- `sb.Write()` にはパスを含めない（ファイル名のみ）
- パスが必要な場合: `sb.Run("mkdir -p pkg")` → `sb.Write("feature.go", ...)` → `sb.Run("mv feature.go pkg/")`
- sandbox は処理完了後に必ず `sb.Close()` で終了させる

## よく使う Make コマンド

```bash
make cluster   # kind クラスタ作成
make deploy    # Agent Pod デプロイ
make logs      # ログ確認
make teardown  # クラスタ削除
```

## feature_list.json

`feature_list.json`（リポジトリルートに配置）はシステム全体の受け入れテスト仕様を管理する JSON ファイルです。

### ファイル構造

```json
[
  {
    "category": "functional",
    "description": "機能の概要説明",
    "steps": [
      "テストステップ 1",
      "テストステップ 2"
    ],
    "passes": false
  }
]
```

### フィールド定義

| フィールド | 型 | 説明 |
| --- | --- | --- |
| `category` | string | `"functional"` / `"integration"` / `"state-management"` / `"configuration"` のいずれか |
| `description` | string | 検証する機能・動作の一文説明 |
| `steps` | string[] | その機能を確認するための手順リスト |
| `passes` | boolean | 実装完了・動作確認済みなら `true`、未実装または未検証なら `false` |

### 実装完了時のルール

ある機能の実装が完了したら、`feature_list.json` 内の対応するエントリの `passes` フィールドを `false` から `true` に更新すること。
