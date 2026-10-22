# 構文解析の依存関係

## 選定

Go標準ライブラリにはMarkdownとYAMLの解析器がないため、次のモジュールを使用します。正規表現だけの自作では、参照形式・入れ子・コード除外と、YAMLの引用・複数行文字列・値の型を正しく区別する負担が大きくなります。HTMLへの変換やYAML値からのオブジェクト生成は行わず、構文木を検査します。

| モジュール | 固定版 | 用途 | 採用理由 |
| --- | --- | --- | --- |
| `github.com/yuin/goldmark` | `v1.8.6` | Markdownのリンク・画像・コードの区別 | CommonMark準拠で、標準ライブラリ以外の実行時依存を持たない安定版。開発中のv2を避ける |
| `go.yaml.in/yaml/v3` | `v3.0.4` | 先頭メタデータの構文と型の確認 | YAML組織が保守するv3の安定API。文字列への暗黙変換を避けて節点の種類を確認できる |

導入時の確認日は2026年10月22日です。固定版とハッシュは`go.mod`・`go.sum`を正本とします。Goのチェックサム検証を無効化せず、取得先を`https://proxy.golang.org`、検証先を`sum.golang.org`に限定します。

## 導入前の確認

| 項目 | 状態 | 根拠・制限 |
| --- | --- | --- |
| 標準機能・既存依存との比較 | 確認済み | [標準ライブラリ一覧](https://pkg.go.dev/std)。導入前の`go.mod`には外部依存なし。簡易解析は上記の構文境界を別途実装する必要がある |
| 正確な名前・保守元・公式取得元 | 確認済み | goldmarkの[版別文書](https://pkg.go.dev/github.com/yuin/goldmark@v1.8.6)と[公式go.mod](https://github.com/yuin/goldmark/blob/v1.8.6/go.mod)、go-yamlの[版別文書](https://pkg.go.dev/go.yaml.in/yaml/v3@v3.0.4)と[公式README](https://github.com/yaml/go-yaml/blob/v3.0.4/README.md)が同じモジュール名を示す |
| 公開時期・保守状況 | 確認済み | 版別文書の公開日はgoldmarkが2026年9月3日、go-yamlが2025年6月29日。公開直後の版ではない。go-yamlは従来の著者の保守終了後、YAML組織へ保守を移管した旨を公式READMEで確認 |
| 所有者・権限の変更履歴すべて | 未確認 | 上記の公表された移管以外に、全管理者の権限履歴は公開資料で確認できない |
| 出所の証明・署名 | 未確認 | 取得物に対応する検証可能な署名・証明を確認していない。署名済みと扱わない |
| 導入処理・ネイティブバイナリ・依存 | 確認済み | 両者は純Goのソース。`go mod download`は生成処理を起動しない。goldmarkは外部依存なし。go-yamlの`go.mod`はテスト用に`gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405`を要求する。製品ではこれを読み込まない。CGOと`go generate`は使用しない |
| 既知の脆弱性・不正配布 | 確認済み（公開情報の範囲） | [goldmarkの公開勧告](https://github.com/yuin/goldmark/security/advisories)・[go-yamlの公開勧告](https://github.com/yaml/go-yaml/security/advisories)に公表なし。未発見の問題や不正コードがないという保証にはしない |
| ライセンス | 確認済み | goldmarkはMIT、go-yamlはファイルごとにMITとApache-2.0。原文は[goldmark](https://github.com/yuin/goldmark/blob/v1.8.6/LICENSE)・[go-yaml](https://github.com/yaml/go-yaml/blob/v3.0.4/LICENSE)を参照。配布時には第三者のライセンス文を同梱する |
| ハッシュと取得物の照合 | 確認済み | 正規プロキシから固定版を取得し、Goのチェックサムデータベースで照合。`go.sum`をGoコマンドで生成し、`go mod verify`で全モジュールの一致を確認 |
| 隔離した取得・ビルド・テスト | 確認済み | 既存GoからLinux用のGoツールを構築し、WSLの専用マウント・プロセス名前空間と非rootの`chroot`で実施。ホストのホームや資格情報は公開しない。取得時だけネットワークを使い、ビルド・テストはネットワーク名前空間も分離。通常・参照形式・空白/日本語の参照、コード除外、数値と複数行文字列の型を最小の検証プログラムで確認した。固定した取得物を使ったWindowsの製品検証も必須スクリプトで確認 |

隔離用の環境は一時領域に置き、製品の依存や通常の検証手順には追加しません。ホストのGoソースは読み取り専用で公開し、書き込みは専用の作業領域・モジュール領域・キャッシュに限ります。初回の候補検証後に、同じ固定済みの取得物をWindowsで使用しました。ライブラリの生成指示は実行せず、製品の実行時にも通信しません。

テスト用の間接依存`gopkg.in/check.v1`は、[公式保守元](https://github.com/go-check/check)と[対象版のライセンス](https://github.com/go-check/check/blob/20d25e280405/LICENSE)を確認し、同じ隔離環境へ固定版で取得しました。BSD-2-Clauseのソースで、製品には組み込みません。署名と管理者権限の全履歴は未確認です。このモジュール自体のテストは起動していません。

製品に組み込む2件の元のLICENSE・NOTICEとApache-2.0の全文は[THIRD_PARTY_NOTICES.txt](../THIRD_PARTY_NOTICES.txt)へ保持します。rulecheck自体のライセンスはこの表示では定めません。
