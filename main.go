package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"log"
	"net/http"
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

	// ISO 8601 (YYYY-MM-DD) をパース
	parsed, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return newErrorResponse(http.StatusBadRequest, ErrorResponse{
			Code:    "INVALID_DATE",
			Message: "date は YYYY-MM-DD 形式で指定してください。",
			Details: map[string]interface{}{"reason": "parse_error", "value": dateStr},
		}), nil
	}

	// シンプルな実装として、週末 (土日) 以外を労働日とみなす。
	// 日本の祝日や振替休日などについては、将来的に専用ライブラリ等で拡張可能。
	isWorkday := !isWeekend(parsed)

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

type Holiday struct {
	Date time.Time `json:"date"`
	Name string    `json:"name"`
}

func parseHolidays() ([]Holiday, error) {
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
	holidays := []Holiday{}
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		return nil, err
	}
	for {
		record, err := reader.Read()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		parsedDate, err := time.ParseInLocation("2006/1/2", record[0], loc)
		if err != nil {
			log.Printf("failed to parse date: %v", err)
			continue
		}
		holidays = append(holidays, Holiday{
			Date: parsedDate,
			Name: record[1],
		})
	}

	return holidays, nil
}

func main() {
	lambda.Start(handleRequest)
}
