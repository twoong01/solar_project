package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCollectPagesAndDiagnostic(t *testing.T) {
	old := kdhcHTTPClient
	defer func() { kdhcHTTPClient = old }()
	calls := 0
	kdhcHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Query().Get("serviceKey") != "fakekey" {
			t.Fatalf("key missing")
		}
		d := req.URL.Query().Get("startDate")
		p := req.URL.Query().Get("pageNo")
		amount := 100
		if p == "2" {
			amount = 3
		}
		var s strings.Builder
		s.WriteString(`<response><header><resultCode>00</resultCode></header><body><items>`)
		for i := 0; i < amount; i++ {
			fmt.Fprintf(&s, `<item><genName>강남</genName><ymd>%s</ymd><workHr>%02d</workHr><accumGen>%d</accumGen><rnum>%d</rnum></item>`, d, i%24, i+10+func() int {
				if p == "2" {
					return 100
				}
				return 0
			}(), i+1+func() int {
				if p == "2" {
					return 100
				}
				return 0
			}())
		}
		s.WriteString(`</items></body></response>`)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(s.String())), Header: http.Header{}}, nil
	})}
	out, csv, err := collectKDHC(context.Background(), "https://apis.data.go.kr/B550373/kdhcSolarPowerProd2/solarPowerProd2", "fakekey", "2026-04-02")
	if err != nil {
		t.Fatalf("collect failed: %s", err)
	}
	if out.Rows != 309 || calls != 6 || len(csv) == 0 {
		t.Fatalf("bad collection: %+v calls=%d csv=%d", out, calls, len(csv))
	}
	if out.MatchedSites != 1 || out.ModelReady || len(out.IncompleteSites) != 12 {
		t.Fatalf("should not use as model inputs: %+v", out)
	}
	if !strings.Contains(string(csv), "genName,ymd,workHr,accumGen,rnum") {
		t.Fatal("CSV header missing")
	}
}
func TestCollectRejectsFuture(t *testing.T) {
	future := time.Now().AddDate(0, 0, 3).Format("2006-01-02")
	_, _, err := collectKDHC(context.Background(), "unused", "unused", future)
	if err == nil || !strings.Contains(err.Error(), "미래") {
		t.Fatalf("expected future-date error: %v", err)
	}
}
