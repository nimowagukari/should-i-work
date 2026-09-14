package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
)

// logger はアプリケーション全体で使用する構造化ロガーです。JSON ハンドラで
// 標準出力に書き出すことで、Lambda の logging_config (log_format = "JSON")
// と合わせて CloudWatch Logs Insights からフィールド単位でクエリ可能になります。
var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

// Middleware は net/http のハンドラを別のハンドラでラップする関数の型です。
type Middleware func(http.Handler) http.Handler

// Chain は複数の Middleware を handler に外側から順に適用します。
// 例: Chain(mux, a, b) は a(b(mux)) を返し、実行順は a → b → mux となります。
func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// loggingResponseWriter は http.ResponseWriter をラップし、後段のアクセスログ用に
// 実際に書き込まれたステータスコードを捕捉します。
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(statusCode int) {
	lrw.statusCode = statusCode
	lrw.ResponseWriter.WriteHeader(statusCode)
}

// scheme はリクエストのスキームを返します。本アプリの *http.Request は
// API Gateway のイベントから合成されたものであり r.TLS は常に nil のため、
// 代わりに API Gateway が付与する X-Forwarded-Proto ヘッダーを参照します。
// ヘッダーが無い場合は、本アプリがカスタムドメイン経由で HTTPS のみを
// 公開している前提で "https" をデフォルトとします。
func scheme(r *http.Request) string {
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		return p
	}
	return "https"
}

// requestIDContextKey は API Gateway のリクエスト ID を context.Context へ
// 格納する際のキー型です。
type requestIDContextKey struct{}

// withRequestID はリクエスト ID を紐付けた context.Context を返します。
// toHTTPRequest がリクエスト変換時に一度だけ呼び出します。
func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, id)
}

// requestIDFromContext は context.Context からリクエスト ID を取り出します。
func requestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

// requestLogAttrsKey は requestLogAttrs を context.Context へ格納する際のキー型です。
type requestLogAttrsKey struct{}

// requestLogAttrs は 1 リクエストあたりのアクセスログに付加する追加フィールドを
// 蓄積する入れ物です。loggingMiddleware がリクエスト処理の最後に一度だけログを
// 出力する設計のため、ビジネスロジック側 (handleWorkdayByDate 等) で判明した
// 結果（判定結果やエラー種別）をここ経由で後から回収します。
// net/http のリクエスト処理は同期的で goroutine をまたがないため、
// 同期化 (mutex 等) は行っていません。
type requestLogAttrs struct {
	attrs []slog.Attr
}

// withRequestLogAttrs は空の requestLogAttrs を紐付けた context.Context を返します。
func withRequestLogAttrs(ctx context.Context) (context.Context, *requestLogAttrs) {
	a := &requestLogAttrs{}
	return context.WithValue(ctx, requestLogAttrsKey{}, a), a
}

// addLogAttrs は現在のリクエストのアクセスログに付加するフィールドを追加します。
// r.Context() に requestLogAttrs が乗っていない場合（テストでハンドラを
// 直接呼び出した場合など）は何もしません。将来 /v1/workdays/range 等の
// 新エンドポイントを追加する際も、この関数を呼ぶだけで共通コードを
// 変更せずに独自フィールドを付加できます。
func addLogAttrs(r *http.Request, attrs ...slog.Attr) {
	if a, ok := r.Context().Value(requestLogAttrsKey{}).(*requestLogAttrs); ok {
		a.attrs = append(a.attrs, attrs...)
	}
}

// loggingMiddleware は OpenTelemetry の semantic conventions に準拠したフィールド名で
// アクセスログを1リクエスト1行出力する net/http ミドルウェアです。
// 参考: https://github.com/nimowagukari/portfolio-golang/blob/main/internal/server/server.go#L145-L185
func loggingMiddleware(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ctx, logAttrs := withRequestLogAttrs(r.Context())
			r = r.WithContext(ctx)
			lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(lrw, r)

			elapsed := time.Since(start)

			attrs := []slog.Attr{
				slog.String("faas.invocation_id", requestIDFromContext(r.Context())),
				slog.String("http.request.method", r.Method),
				slog.Int("http.response.status_code", lrw.statusCode),
				slog.String("url.path", r.URL.Path),
				slog.String("url.scheme", scheme(r)),
				slog.String("server.address", r.Host),
				slog.String("user_agent.original", r.UserAgent()),
				slog.Float64("http.server.request.duration", elapsed.Seconds()),
			}

			client, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				logger.Warn("failed to parse remote address", slog.String("remote_addr", r.RemoteAddr), slog.Any("error", err))
			} else {
				attrs = append(attrs, slog.String("client.address", client))
			}

			attrs = append(attrs, logAttrs.attrs...)
			logger.LogAttrs(r.Context(), slog.LevelInfo, "", attrs...)
		})
	}
}
