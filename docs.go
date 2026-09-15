package main

import "embed"

// docsFS は Swagger UI 一式（index.html, openapi.yaml 等）を Lambda のデプロイパッケージ
// （bootstrap バイナリ単体、付随ファイルなし）へ同梱するための embed.FS です。
// internal/data/embed.go と異なり、埋め込み対象の docs/ ディレクトリはリポジトリ直下
// （このファイルと同じ階層）にあるため、//go:embed ディレクティブもこの階層の
// *.go ファイルに置く必要があります（go:embed はディレクティブを書いたファイルより
// 下の階層しか埋め込めません）。
//
//go:embed docs
var docsFS embed.FS
