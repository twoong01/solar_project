package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSampleSolarOnly(t *testing.T) {
	result, err := loadSample("2025-12-31", "all")
	if err != nil {
		t.Fatal(err)
	}
	if result["fuel"] != "태양광" || result["isRealtime"] != false {
		t.Fatal("wrong data provenance")
	}
	if result["records"].(int) != 408 {
		t.Fatalf("expected 17 regions x 24 rows, got %v", result["records"])
	}
	if !result["complete"].(bool) || len(result["intervals"].([]payload)) != 24 {
		t.Fatal("missing hours")
	}
	if result["totalMWh"].(float64) <= 0 {
		t.Fatal("unexpected empty total")
	}
}
func TestAPIAndStatic(t *testing.T) {
	a := &app{root: t.TempDir(), id: "test", exit: make(chan struct{})}
	for _, p := range []string{"/", "/index.html", "/data.json", "/favicon.ico", "/api/health", "/api/kpx/status", "/api/kpx/sample?date=2025-12-31&region=all"} {
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:55555"+p, nil)
		w := httptest.NewRecorder()
		a.handle(w, req)
		if w.Code != 200 {
			t.Errorf("%s status=%d body=%s", p, w.Code, w.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:55555/api/kpx/auth", strings.NewReader(`{"key":"aaaaaaaaaaaaaaaaaaaaaaaaa"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.handle(w, req)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:55555/api/kpx/status", nil)
	w = httptest.NewRecorder()
	a.handle(w, req)
	var result payload
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result["configured"].(bool) {
		t.Fatal("API key not saved in memory")
	}
	// Browser with external Origin cannot change credentials.
	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:55555/api/kpx/auth", strings.NewReader(`{"key":"aaaaaaaaaaaaaaaa"}`))
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	a.handle(w, req)
	if w.Code != 403 {
		t.Fatalf("cross origin request accepted: %d", w.Code)
	}
}
func TestRejectCrossDataset(t *testing.T) {
	rec := []rawRow{{"연료원": "풍력", "거래일": "2025-12-31", "지역": "울산시", "거래시간": "1", "전력거래량(MWh)": "100"}, {"연료원": "태양광", "거래일": "2025-12-31", "지역": "울산시", "거래시간": "1", "전력거래량(MWh)": "5.2"}}
	out, err := summary(rec, "2025-12-31", "울산시", "테스트")
	if err != nil {
		t.Fatal(err)
	}
	if out["records"].(int) != 1 || out["totalMWh"].(float64) != 5.2 {
		t.Fatalf("data not filtered: %v", out)
	}
}

// The external API can return an HTTP 200 error JSON. Ensure it is shown as
// a provider error, not silently misreported as an unrelated malformed schema.
func TestDiagnoseProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":-401,"msg":"유효하지 않은 인증키 입니다."}`))
	}))
	defer srv.Close()
	old := kpxAPIURL
	kpxAPIURL = srv.URL
	defer func() { kpxAPIURL = old }()
	_, err := diagnoseKPX("test-key")
	if err == nil || !strings.Contains(err.Error(), "유효하지 않은 인증키") {
		t.Fatalf("missing provider error: %v", err)
	}
}
func TestDiagnoseAuthAndSolarRows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if got := r.Header.Get("Authorization"); got != "Infuser test-key" {
			t.Errorf("wrong request authorization: %s", got)
		}
		// A single-row unfiltered probe is valid; filtered date/region gets 24 rows.
		count := 1
		if r.URL.Query().Get("cond[거래일::EQ]") != "" {
			count = 24
		}
		rows := make([]map[string]any, 0, count)
		for i := 1; i <= count; i++ {
			rows = append(rows, map[string]any{"거래일": "2025-12-31", "거래시간": i, "지역": "강원도", "연료원": "태양광", "전력거래량(MWh)": 5.25})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": rows, "matchCount": count})
	}))
	defer srv.Close()
	old := kpxAPIURL
	kpxAPIURL = srv.URL
	defer func() { kpxAPIURL = old }()
	info, err := diagnoseKPX("test-key")
	if err != nil || info["authenticated"] != true {
		t.Fatalf("diagnose error %v: %v", err, info)
	}
	result, err := fetchKPX("2025-12-31", "강원도", "test-key")
	if err != nil || result["records"].(int) != 24 || result["fuel"] != "태양광" {
		t.Fatalf("fetch error %v: %v", err, result)
	}
}
func TestFilteredErrorIsDiagnosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cond[거래일::EQ]") != "" {
			_, _ = w.Write([]byte(`{"code":-400,"msg":"조회조건을 확인하세요"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"거래일":"2025-12-31","연료원":"태양광"}],"matchCount":1}`))
	}))
	defer srv.Close()
	old := kpxAPIURL
	kpxAPIURL = srv.URL
	defer func() { kpxAPIURL = old }()
	_, err := fetchKPX("2023-02-06", "강원도", "test-key")
	if err == nil || !strings.Contains(err.Error(), "조회 조건은 실패") || !strings.Contains(err.Error(), "조회조건을 확인하세요") {
		t.Fatalf("filtered error diagnostic failed %v", err)
	}
}

func TestEmbedded2025SolarCSV(t *testing.T) {
	s, err := offlineData()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Days) != 365 || len(s.Regions) != 17 {
		t.Fatalf("unexpected file %d days %d regions", len(s.Days), len(s.Regions))
	}
	for _, day := range []string{"2025-01-01", "2025-11-24", "2025-12-31"} {
		all, e := loadOfflineSolar(day, "all")
		if e != nil {
			t.Fatal(e)
		}
		if all["records"].(int) != 408 || len(all["intervals"].([]payload)) != 24 || !all["complete"].(bool) {
			t.Fatalf("incomplete %s", day)
		}
		regional, e := loadOfflineSolar(day, "강원도")
		if e != nil {
			t.Fatal(e)
		}
		if regional["records"].(int) != 24 || regional["totalMWh"].(float64) > all["totalMWh"].(float64) {
			t.Fatalf("bad local %s", day)
		}
		if regional["isRealtime"] != false {
			t.Fatal("must not claim real-time")
		}
	}
	if _, e := loadOfflineSolar("2026-11-22", "all"); e == nil {
		t.Fatal("future date was accepted")
	}
}
func TestLocalDashboardEndpoint(t *testing.T) {
	a := &app{root: t.TempDir(), id: "test", exit: make(chan struct{})}
	req := httptest.NewRequest("GET", "http://127.0.0.1:55555/api/kpx/local?date=2025-11-24&region=울산시", nil)
	w := httptest.NewRecorder()
	a.handle(w, req)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var result payload
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["records"].(float64) != 24 || result["fuel"] != "태양광" {
		t.Fatal(result)
	}
}
