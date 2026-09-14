package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
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
