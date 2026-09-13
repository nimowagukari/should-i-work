package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

func TestHandleRequest_Success(t *testing.T) {
	req := events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		Path:       "/v1/workday",
		QueryStringParameters: map[string]string{
			"date": "2024-01-01",
		},
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

func TestHandleRequest_MissingDate(t *testing.T) {
	req := events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		Path:       "/v1/workday",
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

func TestHandleRequest_InvalidDateFormat(t *testing.T) {
	req := events.APIGatewayProxyRequest{
		HTTPMethod: http.MethodGet,
		Path:       "/v1/workday",
		QueryStringParameters: map[string]string{
			"date": "2026/05/03", // 不正形式
		},
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
		Path:       "/v1/workday",
		QueryStringParameters: map[string]string{
			"date": "2024-01-01",
		},
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
		Path:       "/v1/workday",
		QueryStringParameters: map[string]string{
			"date": "2024-02-12",
		},
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
		Path:       "/v1/workday",
		QueryStringParameters: map[string]string{
			"date": "2024-01-04",
		},
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
