# should-i-work

このアプリは、指定した日付が労働日（出勤すべき日）かどうかを判定する単純な HTTP API です。

## エンドポイント

- `GET /v1/workdays/{date}` — 指定日が労働日かどうかを判定する

詳細は [docs/openapi.yaml](docs/openapi.yaml) を参照してください。

## インフラ構成

公開エンドポイントは `https://api.nimowagukari.net/should-i-work/v1/workdays/{date}` です。
カスタムドメイン（`api.nimowagukari.net`、[infras/terraform/modules/apigateway](infras/terraform/modules/apigateway) の `aws_api_gateway_domain_name`）は本リポジトリが所有しており、`base_path_mapping` の `base_path = "should-i-work"` によってこのパス配下にマッピングしています。

**注意:** API Gateway REST API のカスタムドメインは、ステージ名とは異なり `base_path` をプロキシ統合の `event.path` から取り除かずにそのまま Lambda へ転送します（AWS の既知の仕様）。そのため本アプリは `lambda.go` の `basePath`（`"/should-i-work"` 固定）と `stripBasePath` で、このプレフィックスを明示的に除去しています。Terraform 側の `base_path`（`local.app_identifier`）とアプリ側の `basePath` は同じ値に固定しているため、変更する場合は両方を合わせて更新してください。

API ドキュメント（Swagger UI）は `https://api.nimowagukari.net/should-i-work/docs/` で参照できます。静的アセット（`docs/` 配下）は `docs.go` の `//go:embed` によりビルド時にバイナリへ同梱されます。

同じドメインを他のアプリと共有したい場合は、各アプリを別リポジトリ・別 REST API として実装し、`data "aws_api_gateway_domain_name"` で本ドメインを参照した上で、そのアプリ専用の `base_path`（例: `other-app`）で `aws_api_gateway_base_path_mapping` を追加してください。

## 使用方法

[aws-lambda-rie](https://github.com/aws/aws-lambda-runtime-interface-emulator) を利用した検証が可能です。

```bash
# リクエスト内容の編集
vi request_rest.json 
jq '.' request_rest.json
→ エラーが出ないこと

curl -s -d @request_rest.json "localhost:8080/2015-03-31/functions/function/invocations" | jq '.body|=fromjson'
→ エラーが出ないこと
```

## OpenAPI ドキュメントの検証

[docs/openapi.yaml](docs/openapi.yaml) の構文チェック用に、Node の開発ツールを同梱しています（アプリ本体の実行には不要）。

```bash
npm ci
npm run lint:openapi
```