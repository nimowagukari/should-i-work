// Package main は should-i-work アプリケーションのエントリポイントです。
// 指定した日付が労働日（出勤すべき日）かどうかを判定する HTTP API を、
// AWS Lambda (API Gateway REST `{proxy+}` / `ANY` プロキシ統合) 上で提供します。
package main

import (
	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(handleRequest)
}
