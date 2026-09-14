package main

import (
	"encoding/csv"
	"errors"
	"io"
	"log"
	"sync"
	"time"

	"github.com/nimowagukari/should-i-work/internal/data"
)

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

// loadHolidaySet は parseHolidays の結果を初回呼び出し時にのみ取得し、
// 以降はキャッシュを返します。Lambda の実行環境はウォームスタート時に
// 再利用されるため、CSV のパースはコールドスタート時の一度だけで済みます。
var loadHolidaySet = sync.OnceValues(parseHolidays)

// isHoliday は与えられた日付が祝日集合に含まれていれば true を返します。
func isHoliday(t time.Time, holidays map[string]struct{}) bool {
	_, ok := holidays[t.Format("2006-01-02")]
	return ok
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
