package main

import (
	"encoding/json"
	"io/fs"
	"log/slog"
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

// router はアプリケーション全体のハンドラ（ルーティングテーブル + アクセスログ用
// ミドルウェア）を初回呼び出し時にのみ構築し、以降はキャッシュを返します。Lambda の
// 実行環境はウォームスタート時に再利用されるため、構築はコールドスタート時の一度だけで済みます。
var router = sync.OnceValue(newRouter)

// newRouter は本アプリケーションが提供するエンドポイントのルーティングテーブルを構築し、
// loggingMiddleware でラップして返します。新しいエンドポイントを追加する際はこの
// ルーティングテーブルに登録します。
func newRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/workdays/{date}", handleWorkdayByDate) // 指定日が労働日かどうか
	mux.HandleFunc("GET /docs", redirectToDocsIndex)               // 末尾スラッシュなしを補正
	mux.Handle("GET /docs/", newDocsHandler())                     // Swagger UI ドキュメント

	// 上記いずれにも一致しないパスは 404 を返す。
	mux.HandleFunc("/", notFoundHandler)

	return Chain(mux, loggingMiddleware(logger))
}

// handleWorkdayByDate は GET /v1/workdays/{date} のハンドラです。
func handleWorkdayByDate(w http.ResponseWriter, r *http.Request) {
	decision, errResp, status := computeWorkdayDecision(r.PathValue("date"))
	if errResp != nil {
		addLogAttrs(r, slog.String("error.type", errResp.Code))
		writeJSON(w, status, *errResp)
		return
	}
	addLogAttrs(r, slog.String("workday.date", decision.Date), slog.Bool("workday.is_workday", decision.IsWorkday))
	writeJSON(w, status, *decision)
}

// newDocsHandler は埋め込んだ docs/ 配下（Swagger UI 一式）を GET /docs/ 以下として
// 配信する http.Handler を構築します。docsFS は "docs/index.html" のようにディレクトリ名
// を含むパスでファイルを保持しているため、fs.Sub で "docs" を取り除き、
// StripPrefix 適用後の URL パスの残りがそのまま埋め込み FS 内のパスに対応するようにします。
func newDocsHandler() http.Handler {
	sub, err := fs.Sub(docsFS, "docs")
	if err != nil {
		// docs は //go:embed docs によりビルド時に静的に決定される埋め込み対象であり、
		// ビルドが壊れていない限りこのエラーは発生しない。
		panic(err)
	}
	return http.StripPrefix("/docs/", http.FileServerFS(sub))
}

// redirectToDocsIndex は末尾スラッシュなしの GET /docs を GET /docs/ へ補正します。
//
// http.ServeMux は "/docs/" のようなサブツリー登録に対し "/docs" へのリクエストを
// 自動的にリダイレクトしますが、その Location は "/docs/" のようなルート直下絶対パスに
// なります。本アプリは basePath（"/should-i-work"）を stripBasePath で除去した後の
// パスに対してルーティングしているため、その自動リダイレクトをそのまま使うと
// basePath が欠落した誤った Location（api ドメイン直下の /docs/）を返してしまいます。
// そのため、この動作は明示的な GET /docs の登録で上書きし、末尾に "/" を付けるだけの
// 相対パスを Location に直接設定することで、basePath の有無によらず正しい URL へ
// 補正します（net/http.Redirect は相対パスも現在の（basePath 除去後の）パスに対して
// 絶対パス化してしまうため使用しません）。
func redirectToDocsIndex(w http.ResponseWriter, r *http.Request) {
	target := "docs/"
	if q := r.URL.RawQuery; q != "" {
		target += "?" + q
	}
	w.Header().Set("Location", target)
	w.WriteHeader(http.StatusMovedPermanently)
}

// computeWorkdayDecision は指定された日付に基づき労働日判定を行います。
// 成功時は (*WorkdayDecision, nil, http.StatusOK) を、失敗時は (nil, *ErrorResponse, ステータスコード) を返します。
func computeWorkdayDecision(dateStr string) (*WorkdayDecision, *ErrorResponse, int) {
	if dateStr == "" {
		return nil, newInvalidDateError(map[string]interface{}{"reason": "missing"}), http.StatusBadRequest
	}

	loc, err := jstLocation()
	if err != nil {
		logger.Error("failed to load JST location", slog.Any("error", err))
		return nil, &ErrorResponse{Code: "INTERNAL_ERROR", Message: "internal server error"}, http.StatusInternalServerError
	}

	// ISO 8601 (YYYY-MM-DD) を JST の日付としてパース
	parsed, err := time.ParseInLocation(isoDateLayout, dateStr, loc)
	if err != nil {
		return nil, newInvalidDateError(map[string]interface{}{"reason": "parse_error", "value": dateStr}), http.StatusBadRequest
	}

	holidays, err := loadHolidaySet()
	if err != nil {
		logger.Error("failed to load holidays", slog.Any("error", err))
		return nil, &ErrorResponse{Code: "INTERNAL_ERROR", Message: "internal server error"}, http.StatusInternalServerError
	}

	// 土日、および syukujitsu.csv に含まれる祝日（振替休日・国民の休日を含む）以外を労働日とみなす。
	isWorkday := !isWeekend(parsed) && !isHoliday(parsed, holidays)

	return &WorkdayDecision{Date: dateStr, IsWorkday: isWorkday}, nil, http.StatusOK
}

// notFoundHandler はどのルートにも一致しなかったリクエストに対する 404 レスポンスを返します。
func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	addLogAttrs(r, slog.String("error.type", "NOT_FOUND"))
	writeJSON(w, http.StatusNotFound, ErrorResponse{
		Code:    "NOT_FOUND",
		Message: "resource not found",
	})
}

// writeJSON は値を JSON にシリアライズして http.ResponseWriter に書き込みます。
// シリアライズに失敗した場合は、呼び出し元が指定した status に関わらず
// 500 (INTERNAL_ERROR) を返します。
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	body, ok := marshalOrFallback(v)
	if !ok {
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// newInvalidDateError は date パラメータが不正だった場合の ErrorResponse を構築します。
// details には理由 (reason) や実際に渡された値など、失敗要因ごとに異なる情報を渡します。
func newInvalidDateError(details map[string]interface{}) *ErrorResponse {
	return &ErrorResponse{
		Code:    "INVALID_DATE",
		Message: "date は YYYY-MM-DD 形式で指定してください。",
		Details: details,
	}
}

// marshalOrFallback は値を JSON にシリアライズします。シリアライズに失敗した場合は
// ログに記録した上で、固定の INTERNAL_ERROR ボディを表す JSON へフォールバックし、
// 第2戻り値に false を返します（呼び出し元がステータスコードを差し替えるため）。
func marshalOrFallback(v interface{}) ([]byte, bool) {
	body, err := json.Marshal(v)
	if err != nil {
		logger.Error("failed to marshal response", slog.Any("error", err))
		return []byte(`{"code":"INTERNAL_ERROR","message":"internal server error"}`), false
	}
	return body, true
}
