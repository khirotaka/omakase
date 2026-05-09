# Omakase

GitHub Issue を起点に、AI Agent が自律的にコードを実装して Pull Request を作成し、CodeRabbit のレビューフィードバックを受けて修正するまでを自動化するシステムです。

## システム概要

```
GitHub Issue が作成される
       │
       │ GitHub Actions (trigger-agent.yml)
       ▼
  Upstash Redis ─────────────────────▶  Agent Orchestrator (Go)
  (agent-queue)                               │
                                             ├─ agent-sandbox (k8s-sigs)
                                             │   コード実装・テスト
                                             │
                                             └─ GitHub API
                                                 PR 作成
                                                    │
                                             CodeRabbit レビュー
                                                    │ webhook
                                             GitHub Actions
                                          trigger-agent.yml
                                                    │
                                              Upstash Redis ──▶ Agent 修正ループ
```

**特徴**
- ローカルマシンへのインバウンド通信が不要（kind クラスタはポーリング型）
- 外部サービスは Upstash Redis のみ（キュー + セッション管理を兼用）
- GitHub Actions が仲介役となり、Webhook サーバーの運用が不要

## アーキテクチャ

```
┌─────────────────────────────────────────────────────────┐
│  ローカルマシン                                           │
│                                                         │
│  ┌──────────────────────────────────────────────────┐  │
│  │  kind クラスタ                                    │  │
│  │                                                  │  │
│  │  ┌─────────────────┐    ┌────────────────────┐  │  │
│  │  │ Agent Pod        │    │ agent-sandbox Pod  │  │  │
│  │  │                  │    │                    │  │  │
│  │  │  Orchestrator    │───▶│  gVisor (runsc)    │  │  │
│  │  │  (Go)            │    │  git clone         │  │  │
│  │  │                  │    │  コード実装         │  │  │
│  │  │  - Redis Poll    │    │  go test           │  │  │
│  │  │  - LLM 呼び出し  │    │  git push          │  │  │
│  │  │  - Session 管理  │    │                    │  │  │
│  │  │  - GitHub API    │    └────────────────────┘  │  │
│  │  └─────────────────┘                             │  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
          │ RPOP / SET / GET              │ HTTPS
          ▼                              ▼
   Upstash Redis                   Anthropic API
   (Queue + Session)               GitHub API
```

## 処理フロー

### Phase 1: Issue 受信 → 実装

```
1. GitHub Issue 作成
2. GitHub Actions (trigger-agent.yml) が起動
3. Upstash Redis の agent-queue に LPUSH
4. Agent Orchestrator が RPOP でキューを取得
5. セッションを Redis に作成 (status: developing)
6. agent-sandbox を起動
7. LLM (Anthropic API) に issue 内容を渡して実装計画を生成
8. sandbox 内で実装・テストを繰り返す
9. feature ブランチを push
10. GitHub API で PR を作成
11. セッションを更新 (status: review_pending)
```

### Phase 2: CodeRabbit レビュー → 修正

```
12. CodeRabbit が PR をレビュー
13. レビューコメントが付くと GitHub Actions が起動
    (pull_request_review_comment: coderabbitai[bot] を検知)
14. フィードバック内容を Upstash Redis にエンキュー
15. Agent Orchestrator がキューを取得
16. セッションを更新 (status: fixing, iteration: N+1)
17. LLM にフィードバックと既存コードを渡して修正方針を生成
18. sandbox 内で修正・テストを実施
19. ブランチに push (PR は自動更新)
20. iteration が上限 (デフォルト: 5) に達した場合は Issue にコメントして終了
```

## コンポーネント

### Agent Orchestrator

Agent システムの中核となる Go アプリケーションです。

**責務**
- Upstash Redis のポーリング（デフォルト: 30 秒間隔）
- AgentSession のライフサイクル管理
- Anthropic API を通じた LLM 呼び出し
- agent-sandbox の起動・操作・終了
- GitHub API を通じた PR の作成・更新

**AgentSession のステート遷移**

```
[developing] ──▶ [review_pending] ──▶ [fixing] ──▶ [done]
                                         │
                                         └──▶ [aborted]  ← iteration 上限超過
```

### Redis スキーマ

```
# キュー
agent-queue  (List)
  LPUSH payload: {"type": "issue"|"review", "issueNumber": 42, ...}

# セッション
session:{issueNumber}  (String / JSON, TTL: 24h)
  {
    "status":     "developing",
    "branchName": "agent/issue-42",
    "prNumber":   null,
    "iteration":  0,
    "sandboxId":  "xxx-yyy-zzz"
  }
```

### agent-sandbox (kubernetes-sigs/agent-sandbox)

gVisor (runsc) によって隔離された Kubernetes Pod 上でコードを安全に実行する環境です。

| 操作 | 内容 |
|---|---|
| `sb.Run(ctx, "command string")` | シェルコマンドの実行（git, go test など） |
| `sb.Write(ctx, "filename", []byte)` | LLM が生成したコードの書き込み。パスを含む場合は mkdir → Write → mv の3ステップが必要 |
| `sb.Read(ctx, "filename")` | 既存コードの読み込み（LLM への入力） |

**sandbox 内で行われる処理**

```bash
git clone <repo-url> /workspace
# LLM が生成したコードを Write で配置
go build ./...
go test ./...
git add .
git commit -m "feat: implement issue #42"
git push origin agent/issue-42
```

## 技術スタック

| カテゴリ | 技術 | 備考 |
|---|---|---|
| 言語 | Go | Orchestrator 本体 |
| コンテナ | kind | ローカル Kubernetes クラスタ |
| 実行環境 | kubernetes-sigs/agent-sandbox | gVisor による隔離 |
| LLM | Anthropic API (claude-sonnet-4-5) | コード生成・修正方針 |
| キュー | Upstash Redis (List) | HTTP REST API で操作 |
| セッション | Upstash Redis (String/JSON) | キューと兼用 |
| VCS 操作 | sandbox 内 git コマンド | |
| GitHub 連携 | GitHub API (go-github) | PR 作成・更新 |
| レビュー | CodeRabbit | GitHub App としてインストール |

## ディレクトリ構成

```
omakase/
├── agent/
│   ├── main.go               # エントリーポイント・ポーリングループ
│   ├── orchestrator.go       # AgentSession のライフサイクル管理
│   ├── llm.go                # Anthropic API クライアント
│   ├── sandbox.go            # agent-sandbox クライアントラッパー
│   ├── github.go             # GitHub API クライアント
│   └── redis.go              # Upstash Redis クライアント
├── k8s/
│   ├── kind-config.yaml      # kind クラスタ設定
│   ├── agent-deployment.yaml # Agent Pod
│   └── sandbox-rbac.yaml     # agent-sandbox 用 RBAC
├── Dockerfile
├── Makefile
└── README.md
```

## 外部サービス

| サービス | 用途 | 無料枠 |
|---|---|---|
| Upstash Redis | キュー・セッション管理 | 10,000 cmd/day, 256 MB |
| Anthropic API | LLM (コード生成・修正) | 従量課金 |
| GitHub | Issue / PR / Actions | Public リポジトリは無料 |
| CodeRabbit | PR 自動レビュー | OSS は無料 |

**ポーリング頻度について**
デフォルトの 30 秒間隔では約 2,880 cmd/day となり、Upstash 無料枠の範囲に収まります。

## 設定項目

Kubernetes Secret または環境変数で設定します。

```bash
# Anthropic
ANTHROPIC_API_KEY=sk-ant-...

# Upstash Redis
UPSTASH_REDIS_URL=https://xxxxx.upstash.io
UPSTASH_REDIS_TOKEN=...

# GitHub
GITHUB_TOKEN=ghp_...
GITHUB_WEBHOOK_SECRET=...

# Agent 動作設定
POLL_INTERVAL_SEC=30   # ポーリング間隔（秒）
MAX_ITERATION=5        # CodeRabbit フィードバックの最大修正回数
```

## 利用方法

### 1. テンプレートとして新規リポジトリを作成

GitHub の "Use this template" ボタンから新しいリポジトリを作成します。

### 2. 開発対象リポジトリに GitHub Actions を追加

開発対象リポジトリの `.github/workflows/trigger-agent.yml` に以下のワークフローを配置します。

```yaml
name: Trigger Omakase Agent
on:
  issues:
    types: [opened, labeled]
  pull_request_review_comment:
    types: [created]

jobs:
  enqueue:
    runs-on: ubuntu-latest
    if: |
      github.event_name == 'issues' ||
      github.event.comment.user.login == 'coderabbitai[bot]'
    steps:
      - name: Enqueue to Redis
        env:
          UPSTASH_REDIS_URL: ${{ secrets.UPSTASH_REDIS_URL }}
          UPSTASH_REDIS_TOKEN: ${{ secrets.UPSTASH_REDIS_TOKEN }}
        run: |
          curl -X POST "$UPSTASH_REDIS_URL/lpush/agent-queue" \
            -H "Authorization: Bearer $UPSTASH_REDIS_TOKEN" \
            -H "Content-Type: application/json" \
            -d '["{\"type\":\"${{ github.event_name }}\",\"issueNumber\":${{ github.event.issue.number || github.event.pull_request.number }}}"]'
```

### 3. Secret の設定

```bash
kubectl create secret generic agent-secrets \
  --from-literal=ANTHROPIC_API_KEY=sk-ant-... \
  --from-literal=UPSTASH_REDIS_URL=https://... \
  --from-literal=UPSTASH_REDIS_TOKEN=... \
  --from-literal=GITHUB_TOKEN=ghp_...
```

### 4. kind クラスタを起動してデプロイ

```bash
make cluster   # kind クラスタ作成
make deploy    # Agent Pod のデプロイ
make logs      # ログ確認
```

### 5. CodeRabbit をインストール

[coderabbit.ai](https://coderabbit.ai/) から GitHub App をインストールします。OSS（Public リポジトリ）は無料で利用できます。
