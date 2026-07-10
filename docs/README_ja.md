# Monitor

![Badge](https://hitscounter.dev/api/hit?url=https%3A%2F%2Fgithub.com%2Fmonitor-monitor%2Fmonitor&label=&icon=github&color=%23a370f7&message=&style=flat&tz=UTC)

![monitor](https://socialify.git.ci/monitor-monitor/monitor/image?description=1&font=Inter&forks=1&issues=1&language=1&logo=https%3A%2F%2Fraw.githubusercontent.com%2Fmonitor-monitor%2Fmonitor-web%2Fd54ce1288df41ead08aa19f8700186e68028a889%2Fpublic%2Ffavicon.png&name=1&owner=1&pattern=Plus&pulls=1&stargazers=1&theme=Auto)

Monitorは、サーバーのパフォーマンスを監視するためのシンプルで効率的なソリューションを提供することを目的とした、軽量の自己ホスト型サーバー監視ツールです。Webインターフェースを介してサーバーのステータスを表示し、軽量エージェントを介してデータを収集します。

> [!WARNING]
> Monitorは、自己ホスト型の監視/制御プログラムであり、所有しているシステム、または管理権限を得ているシステムでのみ使用してください。Monitorを武器化したり、許可なく展開、アクセス、永続化、コマンド実行、その他の不正利用に使用したりしないでください。実際の悪用リスクについては、Huntressの分析をご参照ください：[Monitor C2 agent abuse](https://www.huntress.com/blog/monitor-c2-agent-abuse)。
> Monitorの展開および運用方法については、利用者自身が責任を負います。開発者は、無許可または不正な利用、およびその結果について責任を負いません。
> Windows端末でリモートコントロールを有効にした場合、クライアントはユーザーがログインするたびに、MonitorがリモートコントロールソフトウェアであることをWindows通知で知らせます。

[ドキュメント](https://monitor-document.pages.dev/) | [Telegramグループ](https://t.me/monitor_monitor)

## 特徴

- **軽量で効率的**: リソース消費が少なく、あらゆる規模のサーバーに適しています。
- **自己ホスト型**: データプライバシーを完全に制御でき、展開も簡単です。
- **Webインターフェース**: 直感的な監視ダッシュボードで、使いやすいです。

## クイックスタート

### 0. クラウドホスティングによるワンクリック展開

- Rainyun - CNY 4.5/月

[![](https://rainyun-apps.cn-nb1.rains3.com/materials/deploy-on-rainyun-cn.svg)](https://app.rainyun.com/apps/rca/store/6780/NzYxNzAz_)

- 1Panel アプリストア

1Panel アプリストアで利用可能です。「アプリストア」>「ユーティリティ」>「Monitor」からインストールしてください。

### 1. ワンクリックインストールスクリプトを使用する

systemdを使用するディストリビューション（Ubuntu、Debianなど）に適しています。

```bash
curl -fsSL https://raw.githubusercontent.com/monitor-monitor/monitor/main/install-monitor.sh -o install-monitor.sh
chmod +x install-monitor.sh
sudo ./install-monitor.sh
```

### 2. Docker展開

1. データディレクトリを作成します:
   ```bash
   mkdir -p ./data
   ```
2. Dockerコンテナを実行します:
   ```bash
   docker run -d \
     -p 25774:25774 \
     -v $(pwd)/data:/app/data \
     --name monitor \
     ghcr.io/monitor-monitor/monitor:latest
   ```
3. デフォルトのユーザー名とパスワードを表示します:
   ```bash
   docker logs monitor
   ```
4. ブラウザで `http://<your_server_ip>:25774` にアクセスします。

> [!NOTE]
> 環境変数 `ADMIN_USERNAME` と `ADMIN_PASSWORD` を使用して、初期のユーザー名とパスワードをカスタマイズすることもできます。

### 3. バイナリファイル展開

1. Monitorの[GitHubリリース](https://github.com/monitor-monitor/monitor/releases)ページにアクセスして、お使いのオペレーティングシステム用の最新のバイナリをダウンロードします。
2. Monitorを実行します:
   ```bash
   ./monitor server -l 0.0.0.0:25774
   ```
3. ブラウザで `http://<your_server_ip>:25774` にアクセスします。デフォルトのポートは `25774` です。
4. デフォルトのユーザー名とパスワードは、起動ログで確認するか、環境変数 `ADMIN_USERNAME` と `ADMIN_PASSWORD` を介して設定できます。

> [!NOTE]
> バイナリに実行権限があることを確認してください（`chmod +x monitor`）。データは実行ディレクトリの `data` フォルダに保存されます。

### 手動ビルド

#### 依存関係

- Go 1.18+ および Node.js 20+（手動ビルド用）

1. フロントエンドの静的ファイルをビルドします:
   ```bash
   git clone https://github.com/monitor-monitor/monitor-web
   cd monitor-web
   npm install
   npm run build
   ```
2. バックエンドをビルドします:
   ```bash
   git clone https://github.com/monitor-monitor/monitor
   cd monitor
   ```
   ステップ1で生成された静的ファイルを `monitor` プロジェクトのルートにある `/web/public/defaultTheme/dist` フォルダにコピーし、`monitor-theme.json` と `preview.png`/`perview.png` を `/web/public/defaultTheme` にコピーします。
   ```bash
   go build -o monitor
   ```
3. 実行:
   ```bash
   ./monitor server -l 0.0.0.0:25774
   ```
   デフォルトのリスニングポートは `25774` です。`http://localhost:25774` にアクセスします。

## フロントエンド開発ガイド

[Monitorテーマ開発ガイド | Monitor](https://monitor-document.pages.dev/dev/theme.html)

[CrowdinでMonitorを翻訳する](https://crowdin.com/project/monitor/invite?h=cd051bf172c9a9f7f1360e87ffb521692507706)

## クライアントエージェント開発ガイド

[Monitorエージェント情報レポートおよびイベント処理ドキュメント](https://monitor-document.pages.dev/dev/agent.html)

## 貢献

IssueやPull Requestを歓迎します！

## 謝辞

### 破碎工坊云 (Crash Work)

[破碎工坊云 - 効率的で安定した、安全な高防御サーバーとCDNソリューションを提供する専門的なクラウドコンピューティングプラットフォーム](https://www.crash.work/)

### DreamCloud

[DreamCloud - コストパフォーマンスに優れたアジア太平洋向け高防御直通](https://as211392.com/)

### 🚀 SharonNetworks スポンサー

[![Sharon Networks](https://raw.githubusercontent.com/monitor-monitor/public/refs/heads/main/images/sharon-networks.webp)](https://sharon.io)

SharonNetworks は、あなたのビジネスの離陸を力強くサポートします！

アジア太平洋のデータセンターから中国最適化ネットワーク接続を提供。低レイテンシー & 高帯域幅、Tbps 級のローカル洗浄 DDoS 防御で、ビジネスとお客様の体験を守ります。コミュニティ [Telegram グループ](https://t.me/SharonNetwork) に参加すると、チャリティまたはグループ内抽選で無料利用のチャンスがあります。

### オープンソースコミュニティ

PR を送ってくれた方、テーマを作成してくれた全ての開発者

—— そして：こんなに暇でいられる自分に感謝

## Star履歴

[![Star History Chart](https://api.star-history.com/svg?repos=monitor-monitor/monitor&type=Date)](https://www.star-history.com/#monitor-monitor/monitor&Date)
