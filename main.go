package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

// router はアプリケーション全体のルーティングテーブルを初回呼び出し時にのみ構築し、
// 以降はキャッシュを返します。Lambda の実行環境はウォームスタート時に再利用されるため、
// テーブルの構築はコールドスタート時の一度だけで済みます。
var router = sync.OnceValue(newRouter)

// newRouter は本アプリケーションが提供するエンドポイントのルーティングテーブルを構築します。
// 新しいエンドポイントを追加する際はこのテーブルに登録します。
func newRouter() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/workdays/{date}", handleWorkdayByDate) // 指定日が労働日かどうか

	// 上記いずれにも一致しないパスは 404 を返す。
	mux.HandleFunc("/", notFoundHandler)

	return mux
}

// handleWorkdayByDate は GET /v1/workdays/{date} のハンドラです。
func handleWorkdayByDate(w http.ResponseWriter, r *http.Request) {
	decision, errResp, status := computeWorkdayDecision(r.PathValue("date"))
	if errResp != nil {
		writeJSON(w, status, *errResp)
		return
	}
	writeJSON(w, status, *decision)
}

// computeWorkdayDecision は指定された日付に基づき労働日判定を行います。
// 成功時は (*WorkdayDecision, nil, http.StatusOK) を、失敗時は (nil, *ErrorResponse, ステータスコード) を返します。
func computeWorkdayDecision(dateStr string) (*WorkdayDecision, *ErrorResponse, int) {
	if dateStr == "" {
		return nil, &ErrorResponse{
			Code:    "INVALID_DATE",
			Message: "date は YYYY-MM-DD 形式で指定してください。",
			Details: map[string]interface{}{"reason": "missing"},
		}, http.StatusBadRequest
	}

	loc, err := jstLocation()
	if err != nil {
		log.Printf("failed to load JST location: %v", err)
		return nil, &ErrorResponse{Code: "INTERNAL_ERROR", Message: "internal server error"}, http.StatusInternalServerError
	}

	// ISO 8601 (YYYY-MM-DD) を JST の日付としてパース
	parsed, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		return nil, &ErrorResponse{
			Code:    "INVALID_DATE",
			Message: "date は YYYY-MM-DD 形式で指定してください。",
			Details: map[string]interface{}{"reason": "parse_error", "value": dateStr},
		}, http.StatusBadRequest
	}

	holidays, err := loadHolidaySet()
	if err != nil {
		log.Printf("failed to load holidays: %v", err)
		return nil, &ErrorResponse{Code: "INTERNAL_ERROR", Message: "internal server error"}, http.StatusInternalServerError
	}

	// 土日、および syukujitsu.csv に含まれる祝日（振替休日・国民の休日を含む）以外を労働日とみなす。
	isWorkday := !isWeekend(parsed) && !isHoliday(parsed, holidays)

	return &WorkdayDecision{Date: dateStr, IsWorkday: isWorkday}, nil, http.StatusOK
}

// notFoundHandler はどのルートにも一致しなかったリクエストに対する 404 レスポンスを返します。
func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, ErrorResponse{
		Code:    "NOT_FOUND",
		Message: "resource not found",
	})
}

// writeJSON は値を JSON にシリアライズして http.ResponseWriter に書き込みます。
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	body, err := json.Marshal(v)
	if err != nil {
		log.Printf("failed to marshal response: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"INTERNAL_ERROR","message":"internal server error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
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
// router() のディスパッチに乗せられない、リクエスト変換自体の失敗時にのみ使用します。
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
