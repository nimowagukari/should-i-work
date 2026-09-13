# should-i-work

このアプリは、指定した日付が労働日（出勤すべき日）かどうかを判定する単純な HTTP API です。

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