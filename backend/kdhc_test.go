package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type doer func(*http.Request) (*http.Response, error)

func (d doer) RoundTrip(r *http.Request) (*http.Response, error) { return d(r) }
func TestKDHCEndpointRestriction(t *testing.T) {
	good := "https://apis.data.go.kr/B550373/example/getSolar"
	if _, e := validateKDHCEndpoint(good); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{"http://apis.data.go.kr/a", "https://evil.com/x", "https://apis.data.go.kr/a?serviceKey=secret", "https://127.0.0.1/x", "https://apis.data.go.kr/"} {
		if _, e := validateKDHCEndpoint(bad); e == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
func TestKDHCXMLAndModelGate(t *testing.T) {
	p := []byte(`<response><header><resultCode>00</resultCode><resultMsg>NORMAL_SERVICE</resultMsg></header><body><items><item><genName>판교(가압장)</genName><ymd>20230811</ymd><workHr>23</workHr><accumGen>410288</accumGen></item><item><genName>강남</genName><ymd>20230811</ymd><workHr>12</workHr><accumGen>101</accumGen></item></items></body></response>`)
	d, _, e := parseKDHCPayload(p)
	if e != nil {
		t.Fatal(e)
	}
	if d.TotalItems != 2 || len(d.MatchedSites) != 2 || d.ModelReady {
		t.Fatalf("bad result %+v", d)
	}
	if d.Items[0]["accumGen"] != "410288" {
		t.Fatal(d.Items)
	}
}
func TestKDHCOfficialAPIError(t *testing.T) {
	_, _, e := parseKDHCPayload([]byte(`<response><header><resultCode>30</resultCode><resultMsg>SERVICE_KEY_IS_NOT_REGISTERED_ERROR</resultMsg></header></response>`))
	if e == nil || !strings.Contains(e.Error(), "30") {
		t.Fatal(e)
	}
}
func TestKDHCRequestKeyNeverReturned(t *testing.T) {
	old := kdhcHTTPClient
	defer func() { kdhcHTTPClient = old }()
	const secret = "DUMMY_SUPER_SECRET_FAKE_KEY"
	kdhcHTTPClient = &http.Client{Transport: doer(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("serviceKey") != secret {
			return nil, errors.New("missing secret")
		}
		if r.URL.Query().Get("startDate") != "20230811" {
			t.Fatal("missing startDate")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`<response><header><resultCode>30</resultCode><resultMsg>DENIED</resultMsg></header></response>`)), Header: http.Header{}}, nil
	})}
	_, e := queryKDHC(context.Background(), "https://apis.data.go.kr/B550373/svc/op", secret, "20230811", "20230811", 100)
	if e == nil || strings.Contains(e.Error(), secret) {
		t.Fatal(e)
	}
}
