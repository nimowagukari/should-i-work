package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/nimowagukari/should-i-work/internal/data"
)

// WorkdayDecision は OpenAPI の WorkdayDecision スキーマに対応するレスポンスボディです。
type WorkdayDecision struct {
	Date      string `json:"date"`
	IsWorkday bool   `json:"isWorkday"`
}

// ErrorResponse は OpenAPI の Error スキーマに対応するレスポンスボディです。
type ErrorResponse struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// handleRequest は API Gateway (REST) からのリクエストを受け取り、
// `/v1/workday` GET エンドポイントに対するレスポンスを返します。
func handleRequest(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// ルーティング: 今回は 1 エンドポイントのみなので、簡易なチェックにとどめる
	if req.Path != "/v1/workday" || req.HTTPMethod != http.MethodGet {
		return newErrorResponse(http.StatusNotFound, ErrorResponse{
			Code:    "NOT_FOUND",
			Message: "resource not found",
		}), nil
	}

	dateStr, ok := req.QueryStringParameters["date"]
	if !ok || dateStr == "" {
		return newErrorResponse(http.StatusBadRequest, ErrorResponse{
			Code:    "INVALID_DATE",
			Message: "date は YYYY-MM-DD 形式で指定してください。",
			Details: map[string]interface{}{"reason": "missing"},
		}), nil
	}

	loc, err := jstLocation()
	if err != nil {
		log.Printf("failed to load JST location: %v", err)
		return newErrorResponse(http.StatusInternalServerError, ErrorResponse{
			Code:    "INTERNAL_ERROR",
			Message: "internal server error",
		}), nil
	}

	// ISO 8601 (YYYY-MM-DD) を JST の日付としてパース
	parsed, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		return newErrorResponse(http.StatusBadRequest, ErrorResponse{
			Code:    "INVALID_DATE",
			Message: "date は YYYY-MM-DD 形式で指定してください。",
			Details: map[string]interface{}{"reason": "parse_error", "value": dateStr},
		}), nil
	}

	holidays, err := loadHolidaySet()
	if err != nil {
		log.Printf("failed to load holidays: %v", err)
		return newErrorResponse(http.StatusInternalServerError, ErrorResponse{
			Code:    "INTERNAL_ERROR",
			Message: "internal server error",
		}), nil
	}

	// 土日、および syukujitsu.csv に含まれる祝日（振替休日・国民の休日を含む）以外を労働日とみなす。
	isWorkday := !isWeekend(parsed) && !isHoliday(parsed, holidays)

	body, err := json.Marshal(WorkdayDecision{
		Date:      dateStr,
		IsWorkday: isWorkday,
	})
	if err != nil {
		log.Printf("failed to marshal WorkdayDecision: %v", err)
		return newErrorResponse(http.StatusInternalServerError, ErrorResponse{
			Code:    "INTERNAL_ERROR",
			Message: "internal server error",
		}), nil
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// isWeekend は与えられた日付が土日であれば true を返します。
func isWeekend(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// jstLocation は Asia/Tokyo の time.Location を初回呼び出し時にのみロードし、
// 以降はキャッシュを返します。日付は本アプリケーション全体で JST として
// 解釈するため、この関数を唯一の取得経路とします。
var jstLocation = sync.OnceValues(func() (*time.Location, error) {
	return time.LoadLocation("Asia/Tokyo")
})

var (
	holidaysOnce sync.Once
	holidaySet   map[string]struct{}
	holidaysErr  error
)

// loadHolidaySet は parseHolidays の結果を初回呼び出し時にのみ取得し、
// 以降はキャッシュを返します。Lambda の実行環境はウォームスタート時に
// 再利用されるため、CSV のパースはコールドスタート時の一度だけで済みます。
func loadHolidaySet() (map[string]struct{}, error) {
	holidaysOnce.Do(func() {
		holidaySet, holidaysErr = parseHolidays()
	})
	return holidaySet, holidaysErr
}

// isHoliday は与えられた日付が祝日集合に含まれていれば true を返します。
func isHoliday(t time.Time, holidays map[string]struct{}) bool {
	_, ok := holidays[t.Format("2006-01-02")]
	return ok
}

// newErrorResponse は ErrorResponse を JSON にシリアライズして返します。
func newErrorResponse(status int, errBody ErrorResponse) events.APIGatewayProxyResponse {
	body, err := json.Marshal(errBody)
	if err != nil {
		log.Printf("failed to marshal ErrorResponse: %v", err)
		// 最終手段として、素のテキストを返す
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       `{"code":"INTERNAL_ERROR","message":"internal server error"}`,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}
	}

	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}
}

// parseHolidays は syukujitsu.csv をパースし、祝日（振替休日・国民の休日を含む）の
// 日付文字列(YYYY-MM-DD)集合を返します。
func parseHolidays() (map[string]struct{}, error) {
	csvFile, err := data.CsvFS.Open("csv/syukujitsu.csv")
	if err != nil {
		return nil, err
	}
	defer csvFile.Close()

	reader := csv.NewReader(csvFile)
	// ヘッダー行をスキップ
	if _, err := reader.Read(); err != nil {
		return nil, err
	}

	loc, err := jstLocation()
	if err != nil {
		return nil, err
	}

	holidays := make(map[string]struct{})
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		parsedDate, err := time.ParseInLocation("2006/1/2", record[0], loc)
		if err != nil {
			log.Printf("failed to parse date %q: %v", record[0], err)
			continue
		}
		holidays[parsedDate.Format("2006-01-02")] = struct{}{}
	}

	return holidays, nil
}

func main() {
	lambda.Start(handleRequest)
}
