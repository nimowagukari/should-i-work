package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

func TestHandleRequest_Success(t *testing.T) {
	req := events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		Path:       "/v1/workdays/2024-01-01",
	}

	resp, err := handleRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("handleRequest returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body WorkdayDecision
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	if body.Date != "2024-01-01" {
		t.Errorf("unexpected date: got %s, want %s", body.Date, "2024-01-01")
	}
}

func TestHandleRequest_InvalidDateFormat(t *testing.T) {
	req := events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		Path:       "/v1/workdays/2026-13-45", // 不正形式（存在しない月日）
	}

	resp, err := handleRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("handleRequest returned error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected status code: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var body ErrorResponse
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	if body.Code != "INVALID_DATE" {
		t.Errorf("unexpected error code: got %s, want %s", body.Code, "INVALID_DATE")
	}
}

func TestHandleRequest_NotFound(t *testing.T) {
	req := events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		Path:       "/unknown",
	}

	resp, err := handleRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("handleRequest returned error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected status code: got %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	var body ErrorResponse
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	if body.Code != "NOT_FOUND" {
		t.Errorf("unexpected error code: got %s, want %s", body.Code, "NOT_FOUND")
	}
}

func TestHandleRequest_WithBasePath(t *testing.T) {
	// api gateway の base_path_mapping 経由で本番相当のプレフィックス付きパスが
	// 渡ってきた場合でもルーティングできることを確認する。
	req := events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		Path:       "/should-i-work/v1/workdays/2024-01-01",
	}

	resp, err := handleRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("handleRequest returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: got %d, want %d (body=%s)", resp.StatusCode, http.StatusOK, resp.Body)
	}

	var body WorkdayDecision
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	if body.Date != "2024-01-01" {
		t.Errorf("unexpected date: got %s, want %s", body.Date, "2024-01-01")
	}
}

func TestHandleRequest_Docs_Index(t *testing.T) {
	req := events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		Path:       "/should-i-work/docs/",
	}

	resp, err := handleRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("handleRequest returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Headers["Content-Type"]; !strings.HasPrefix(ct, "text/html") {
		t.Errorf("unexpected content-type: got %q", ct)
	}
	if !strings.Contains(resp.Body, "swagger-ui") {
		t.Errorf("expected body to contain swagger-ui markup")
	}
}

func TestHandleRequest_Docs_Asset(t *testing.T) {
	req := events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		Path:       "/should-i-work/docs/openapi.yaml",
	}

	resp, err := handleRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("handleRequest returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if !strings.Contains(resp.Body, "openapi:") {
		t.Errorf("expected body to contain the openapi.yaml contents")
	}
}

func TestHandleRequest_Docs_NoTrailingSlashRedirect(t *testing.T) {
	// basePath を含んだ状態で末尾スラッシュなしにアクセスした場合、Location が
	// "/docs/" のような basePath 欠落の絶対パスではなく、basePath を保持できる
	// 相対パスであることを確認する。
	req := events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		Path:       "/should-i-work/docs",
	}

	resp, err := handleRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("handleRequest returned error: %v", err)
	}
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("unexpected status code: got %d, want %d", resp.StatusCode, http.StatusMovedPermanently)
	}
	if got, want := resp.Headers["Location"], "docs/"; got != want {
		t.Errorf("unexpected Location: got %q, want %q", got, want)
	}
}

func TestHandleRequest_Docs_WithoutBasePath(t *testing.T) {
	req := events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		Path:       "/docs/",
	}

	resp, err := handleRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("handleRequest returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestStripBasePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"プレフィックス付きは除去", "/should-i-work/v1/workdays/2024-01-01", "/v1/workdays/2024-01-01"},
		{"プレフィックスのみのパスはルートになる", "/should-i-work", "/"},
		{"プレフィックスなしはそのまま", "/v1/workdays/2024-01-01", "/v1/workdays/2024-01-01"},
		{"部分一致（別名）はプレフィックスとみなさない", "/should-i-workers/x", "/should-i-workers/x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripBasePath(tt.path); got != tt.want {
				t.Errorf("stripBasePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsWeekend(t *testing.T) {
	sat := time.Date(1970, 1, 3, 0, 0, 0, 0, time.UTC) // 土曜日
	sun := time.Date(1970, 1, 4, 0, 0, 0, 0, time.UTC) // 日曜日
	mon := time.Date(1970, 1, 5, 0, 0, 0, 0, time.UTC) // 月曜日

	if !isWeekend(sat) {
		t.Errorf("expected Saturday to be weekend")
	}
	if !isWeekend(sun) {
		t.Errorf("expected Sunday to be weekend")
	}
	if isWeekend(mon) {
		t.Errorf("expected Monday not to be weekend")
	}
}

func TestParseDate(t *testing.T) {
	holidays, err := parseHolidays()
	if err != nil {
		t.Fatalf("parseHolidays returned error: %v", err)
	}
	if _, ok := holidays["2024-01-01"]; !ok {
		t.Errorf("expected 2024-01-01 (元日) to be included in holidays")
	}
}

func TestHandleRequest_Holiday(t *testing.T) {
	// 2024-01-01 は元日（月曜日）で祝日かつ平日
	req := events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		Path:       "/v1/workdays/2024-01-01",
	}

	resp, err := handleRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("handleRequest returned error: %v", err)
	}

	var body WorkdayDecision
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	if body.IsWorkday {
		t.Errorf("expected 2024-01-01 (元日) to not be a workday")
	}
}

func TestHandleRequest_SubstituteHoliday(t *testing.T) {
	// 2024-02-12 は建国記念の日(2/11)の振替休日（月曜日）
	req := events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		Path:       "/v1/workdays/2024-02-12",
	}

	resp, err := handleRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("handleRequest returned error: %v", err)
	}

	var body WorkdayDecision
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	if body.IsWorkday {
		t.Errorf("expected 2024-02-12 (振替休日) to not be a workday")
	}
}

func TestHandleRequest_PlainWeekday(t *testing.T) {
	// 2024-01-04 は木曜日で祝日でも週末でもない
	req := events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		Path:       "/v1/workdays/2024-01-04",
	}

	resp, err := handleRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("handleRequest returned error: %v", err)
	}

	var body WorkdayDecision
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	if !body.IsWorkday {
		t.Errorf("expected 2024-01-04 to be a workday")
	}
}
