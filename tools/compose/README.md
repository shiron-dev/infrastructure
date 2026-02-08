# Compose Manage Tool (cmt)

Docker Compose プロジェクトのための、Source of Truth・プッシュ型同期ツールです。  
`docker compose` 環境向けのプッシュモデル ArgoCD のようなものです。

## 仕組み

`cmt` はローカルリポジトリからプロジェクト定義とホスト設定を読み取り、
SSH/SFTP 経由でリモートホストにプッシュします。
Terraform と同様の **plan / apply** ワークフローに従います。

```
cmt plan   — 変更内容を表示（読み取り専用）
cmt apply  — 変更を適用（確認あり、--auto-approve で省略可）
```

## ディレクトリ構成

cmt 設定の `basePath` で指定する compose ルートディレクトリは、
以下の構成を前提とします:

```
compose/
├── projects/
│   └── <project>/
│       ├── compose.yml          # 共通の compose 定義
│       └── files/               # compose.yml と同じディレクトリにコピーされるファイル
│           └── ...
├── hosts/
│   └── <hostname>/
│       ├── host.yml             # ホストレベルのデフォルト設定・プロジェクト別上書き
│       └── <project>/
│           ├── compose.override.yml   # ホスト固有の compose override
│           ├── .env                   # ホスト固有の環境変数
│           └── files/                 # ホスト固有のファイル（プロジェクトの files/ を上書き）
│               └── ...
└── README.md
```

### リモート側の配置

各プロジェクトのファイルは `<remotePath>/<project>/` に配置されます:

```
/opt/compose/grafana/
├── compose.yml
├── compose.override.yml
├── .env
├── grafana.ini          (files/ から)
└── .cmt-manifest.json   (cmt が管理)
```

## 設定ファイル

### cmt 設定 (`--config`)

```yaml
basePath: ../compose            # compose ルートへのパス（設定ファイルからの相対パス）

defaults:                       # 最低優先度のデフォルト値
  remotePath: /opt/compose
  postSyncCommand: docker compose up -d

hosts:
  - name: server1               # hosts/<hostname>/ ディレクトリ名と一致させる
    host: 192.168.1.10
    user: deploy
    sshAgent: true
    # sshKeyPath: ~/.ssh/id_ed25519.pub
```

### host.yml (`hosts/<hostname>/host.yml`)

```yaml
sshConfig: ../../ssh_config     # SSH config ファイルのパス（host.yml からの相対パス）

remotePath: /srv/compose        # cmt デフォルトをこのホスト用に上書き
postSyncCommand: docker compose up -d

projects:                       # プロジェクト別の上書き
  grafana:
    postSyncCommand: >-
      docker compose -f compose.yml -f compose.override.yml up -d
    dirs:                       # Docker ボリューム用ディレクトリの事前作成
      - grafana_storage
      - grafana_conf
```

#### `sshConfig` — SSH config による接続設定の解決

cmt は常に `ssh -G <host>` を実行して SSH 接続パラメータ
（hostname, user, port, identity file, proxy command 等）を解決します。
解決された値は cmt 設定の YAML 値を**上書き**します。

`sshConfig` を指定した場合は `ssh -G -F <path> <host>` が実行され、
指定した SSH config ファイルが使われます。
未指定の場合はデフォルトの SSH config（`~/.ssh/config` 等）が使われます。

`ssh -G` による解決のため、SSH config の `Match`, `ProxyCommand`, `ProxyJump`,
`IdentityFile` などの高度な機能がそのまま利用できます。

- パスは `host.yml` があるディレクトリ (`hosts/<hostname>/`) からの相対パスで指定します
- cmt 設定の `host` フィールドが `ssh -G` の引数（SSH destination）として使われます

#### `dirs` — ボリュームディレクトリの事前作成

`dirs` にはリモートのプロジェクトディレクトリからの相対パスを指定します。
`cmt apply` 時にファイル同期より先にディレクトリを作成します。
Docker Compose の bind mount 用ディレクトリを事前に用意する用途を想定しています。

```yaml
projects:
  grafana:
    dirs:
      - grafana_storage    # → <remotePath>/grafana/grafana_storage/
      - grafana_conf       # → <remotePath>/grafana/grafana_conf/
```

`cmt plan` では各ディレクトリの状態（`create` / `exists`）が表示されます。

### デフォルト値の解決順序

1. cmt 設定の `defaults`
2. `host.yml` のトップレベルフィールド
3. `host.yml` の `projects.<name>` フィールド

後の設定が前の設定を上書きします。

## CLI リファレンス

```
cmt [--config <path>] <command> [flags]

コマンド:
  plan      変更内容を表示（変更は行わない）
  apply     リモートホストに変更を適用
  schema    設定ファイルの JSON Schema を生成

グローバルフラグ:
  --config  cmt 設定ファイルのパス（デフォルト: config.yml）

plan / apply フラグ:
  --host      ホスト名でフィルタ（複数指定可）
  --project   プロジェクト名でフィルタ（複数指定可）

apply フラグ:
  --auto-approve  確認プロンプトをスキップ

schema:
  cmt schema cmt    cmt 設定の JSON Schema を出力
  cmt schema host   host.yml の JSON Schema を出力
```

## JSON Schema

スキーマは Go の構造体から自動生成されるため、コードとの乖離がありません:

```bash
cmt schema cmt  > schemas/cmt-config.schema.json
cmt schema host > schemas/host-config.schema.json
```

エディタでのバリデーションや補完に利用できます
（例: VS Code の YAML 拡張で `# yaml-language-server` コメントを指定）。

## ビルド

```bash
cd tools/compose
go build -o cmt .
```
