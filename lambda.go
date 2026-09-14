package main

import (
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

// handleRequest は API Gateway (REST, `{proxy+}` / `ANY`) からのリクエストイベントを
// 標準の net/http リクエストへ変換した上で router() にディスパッチし、その結果を
// API Gateway 向けのレスポンスへ変換して返します。
//
// パス毎の実際の処理内容は router() が持つルーティングテーブルに集約されており、
// 新しいエンドポイントを追加する際はここではなく newRouter() を変更します。
func handleRequest(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	httpReq, err := toHTTPRequest(ctx, req)
	if err != nil {
		log.Printf("failed to build http request: %v", err)
		return newErrorResponse(http.StatusInternalServerError, ErrorResponse{
			Code:    "INTERNAL_ERROR",
			Message: "internal server error",
		}), nil
	}

	rec := httptest.NewRecorder()
	router().ServeHTTP(rec, httpReq)

	return events.APIGatewayProxyResponse{
		StatusCode: rec.Code,
		Headers:    flattenHeaders(rec.Header()),
		Body:       rec.Body.String(),
	}, nil
}

// toHTTPRequest は API Gateway のリクエストイベントを、net/http.ServeMux で
// ディスパッチ可能な *http.Request に変換します。
func toHTTPRequest(ctx context.Context, req events.APIGatewayProxyRequest) (*http.Request, error) {
	query := url.Values{}
	for k, v := range req.QueryStringParameters {
		query.Set(k, v)
	}
	reqURL := url.URL{Path: req.Path, RawQuery: query.Encode()}

	return http.NewRequestWithContext(ctx, req.HTTPMethod, reqURL.String(), strings.NewReader(req.Body))
}

// flattenHeaders は http.Header (1キーに複数値を許容) を、API Gateway REST の
// プロキシ統合レスポンスが要求する 1 キー 1 値の map[string]string に変換します。
func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}

// newErrorResponse は ErrorResponse を JSON にシリアライズして返します。
// router() のディスパッチに乗せられない、リクエスト変換自体の失敗時にのみ使用します。
// シリアライズに失敗した場合は、呼び出し元が指定した status に関わらず
// 500 (INTERNAL_ERROR) を返します。
func newErrorResponse(status int, errBody ErrorResponse) events.APIGatewayProxyResponse {
	body, ok := marshalOrFallback(errBody)
	if !ok {
		status = http.StatusInternalServerError
	}

	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}
}
