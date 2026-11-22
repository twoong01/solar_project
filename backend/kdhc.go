package main

// Korea District Heating Corporation inverter solar API: independently query and
// inspect the fields. This does NOT silently reinterpret them as model inputs.
import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var kdhcMemory struct {
	sync.RWMutex
	key      string
	endpoint string
	verified bool
}
var kdhcHTTPClient = &http.Client{Timeout: 22 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
	return errors.New("API 리디렉션은 허용하지 않습니다")
}}
var kdhcSites = []string{"강남", "강릉", "광교", "광양", "기흥", "분당", "세종", "수원(장안)", "양산", "파주", "판교", "함백", "화성"}

type kdhcResponse struct {
	Items        []map[string]string `json:"items"`
	TotalItems   int                 `json:"totalItems"`
	Names        []string            `json:"names"`
	MatchedSites []string            `json:"matchedSites"`
	LatestDate   string              `json:"latestDate"`
	Source       string              `json:"source"`
	Sampled      bool                `json:"sampled"`
	ModelReady   bool                `json:"modelReady"`
	ModelNote    string              `json:"modelNote"`
}

func validateKDHCEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		return "", errors.New("https://apis.data.go.kr/... 형식의 공식 요청 URL만 입력하세요 (쿼리스트링 제외)")
	}
	host := strings.ToLower(u.Hostname())
	if host != "apis.data.go.kr" && host != "www.kdhc.co.kr" {
		return "", errors.New("공공데이터포털 또는 한국지역난방공사 공식 호스트만 허용합니다")
	}
	if !strings.HasPrefix(u.Path, "/") || u.Path == "/" || strings.Contains(u.Path, "..") {
		return "", errors.New("공식 API의 완전한 요청 경로를 입력하세요")
	}
	return u.String(), nil
}
func parseKDHCDay(s string) (time.Time, error) {
	if len(s) != 8 {
		return time.Time{}, errors.New("날짜는 YYYYMMDD 8자리여야 합니다")
	}
	t, e := time.Parse("20060102", s)
	if e != nil || t.Format("20060102") != s {
		return time.Time{}, errors.New("날짜 형식이 올바르지 않습니다")
	}
	return t, nil
}
func cleanKDHCErr(s, key string) string {
	if key != "" {
		s = strings.ReplaceAll(s, key, "[인증키 숨김]")
	}
	if len(s) > 400 {
		s = s[:400] + "…"
	}
	return strings.TrimSpace(s)
}
func parseKDHCPayload(data []byte) (kdhcResponse, string, error) {
	d := xml.NewDecoder(strings.NewReader(string(data)))
	result := kdhcResponse{Items: []map[string]string{}, Source: "한국지역난방공사 · 인버터별 태양광발전 실적 API", Sampled: true, ModelReady: false, ModelNote: "기존 13개 발전소·28개 인버터 식별자, 누적량 차분 기준, 원자료 단위가 확인되기 전에는 GRU/TFT 입력으로 사용할 수 없습니다."}
	var code, msg string
	var inItem bool
	var currentField string
	var item map[string]string
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, "", fmt.Errorf("XML 응답을 파싱할 수 없습니다: %w", err)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			tag := v.Name.Local
			if strings.EqualFold(tag, "item") {
				inItem = true
				item = map[string]string{}
				currentField = ""
			} else if inItem {
				currentField = tag
			} else {
				currentField = tag
			}
		case xml.CharData:
			text := strings.TrimSpace(string(v))
			if text == "" {
				continue
			}
			if inItem && currentField != "" {
				item[currentField] = strings.TrimSpace(item[currentField] + " " + text)
			}
			if strings.EqualFold(currentField, "resultCode") || strings.EqualFold(currentField, "returnReasonCode") {
				code += text
			}
			if strings.EqualFold(currentField, "resultMsg") || strings.EqualFold(currentField, "errMsg") || strings.EqualFold(currentField, "returnAuthMsg") {
				msg += text
			}
		case xml.EndElement:
			tag := v.Name.Local
			if strings.EqualFold(tag, "item") && inItem {
				inItem = false
				result.TotalItems++
				if len(result.Items) < 2000 {
					result.Items = append(result.Items, item)
				}
				currentField = ""
			} else if strings.EqualFold(currentField, tag) {
				currentField = ""
			}
		}
	}
	if code != "" && code != "00" && code != "0" && !strings.EqualFold(code, "NORMAL_SERVICE") {
		return result, "", fmt.Errorf("공식 API 오류 코드 %s: %s", code, msg)
	}
	if result.TotalItems == 0 {
		if msg != "" {
			return result, "", errors.New("공식 API 응답: " + msg)
		}
		return result, "", errors.New("인버터 데이터 항목(item)이 없습니다. 이 날짜에 공개 자료가 없거나 다른 요청 URL일 수 있습니다")
	}
	nameSet := map[string]bool{}
	siteSet := map[string]bool{}
	for _, row := range result.Items {
		name := strings.TrimSpace(valueIgnoreCase(row, "genName"))
		if name != "" {
			nameSet[name] = true
		}
		date := valueIgnoreCase(row, "ymd")
		if date > result.LatestDate {
			result.LatestDate = date
		}
		for _, s := range kdhcSites {
			if name == s || strings.HasPrefix(name, s+"(") || strings.HasPrefix(name, s+"（") {
				siteSet[s] = true
			}
		}
	}
	for n := range nameSet {
		result.Names = append(result.Names, n)
	}
	sort.Strings(result.Names)
	for _, s := range kdhcSites {
		if siteSet[s] {
			result.MatchedSites = append(result.MatchedSites, s)
		}
	}
	if result.TotalItems >= 80 {
		result.ModelNote += " 현재 표시는 첫 80개 표본이며 API 전체 자료 검증이 필요합니다."
	}
	return result, "", nil
}
func valueIgnoreCase(m map[string]string, k string) string {
	for key, v := range m {
		if strings.EqualFold(key, k) {
			return v
		}
	}
	return ""
}
func queryKDHC(ctx context.Context, endpoint, key, start, end string, num int) (kdhcResponse, error) {
	var empty kdhcResponse
	u, err := url.Parse(endpoint)
	if err != nil {
		return empty, err
	}
	q := u.Query()
	q.Set("serviceKey", key)
	q.Set("startDate", start)
	q.Set("endDate", end)
	q.Set("pageNo", "1")
	q.Set("numOfRows", strconv.Itoa(num))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return empty, errors.New("API 요청 생성 실패")
	}
	req.Header.Set("Accept", "application/xml,text/xml,*/*")
	resp, err := kdhcHTTPClient.Do(req)
	if err != nil {
		return empty, errors.New("공식 API 접속 실패: " + cleanKDHCErr(err.Error(), key))
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return empty, errors.New("API 응답 읽기 실패")
	}
	parsed, _, parseErr := parseKDHCPayload(b)
	if resp.StatusCode != 200 {
		// Never include the requested URL, which holds the API key.
		if parseErr != nil {
			return empty, fmt.Errorf("공식 API HTTP %d: %s", resp.StatusCode, cleanKDHCErr(parseErr.Error(), key))
		}
		return empty, fmt.Errorf("공식 API HTTP %d (인증 권한 또는 요청 형식 확인)", resp.StatusCode)
	}
	if parseErr != nil {
		return empty, errors.New(cleanKDHCErr(parseErr.Error(), key))
	}
	return parsed, nil
}
func (a *app) handleKDHC(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/kdhc/collect", "/api/kdhc/last.csv":
		a.handleKDHCCollect(w, r)
		return
	case "/api/kdhc/status":
		if r.Method != "GET" {
			errorJSON(w, 405, errors.New("GET만 허용"))
			return
		}
		kdhcMemory.RLock()
		status := payload{"configured": kdhcMemory.key != "" && kdhcMemory.endpoint != "", "hasKey": kdhcMemory.key != "", "endpoint": kdhcMemory.endpoint, "verified": kdhcMemory.verified, "source": "한국지역난방공사 발전소별 인버터 실적 · XML", "modelReady": false}
		kdhcMemory.RUnlock()
		sendJSON(w, 200, status)
		return
	case "/api/kdhc/config":
		if r.Method != "POST" {
			errorJSON(w, 405, errors.New("POST만 허용"))
			return
		}
		d, e := readBody(r, 12000)
		if e != nil {
			errorJSON(w, 400, e)
			return
		}
		key, _ := d["key"].(string)
		endpoint, _ := d["endpoint"].(string)
		endpoint, e = validateKDHCEndpoint(endpoint)
		if e != nil {
			errorJSON(w, 400, e)
			return
		}
		key = strings.TrimSpace(key)
		if len(key) < 16 || len(key) > 1024 || strings.ContainsAny(key, " \t\r\n") {
			errorJSON(w, 400, errors.New("공공데이터포털 인증키 형식을 확인하세요"))
			return
		}
		kdhcMemory.Lock()
		kdhcMemory.key = key
		kdhcMemory.endpoint = endpoint
		kdhcMemory.verified = false
		kdhcMemory.Unlock()
		sendJSON(w, 200, payload{"ok": true, "message": "키와 공식 호출 URL을 메모리에 등록했습니다. 실제 인증 여부는 조회 후 확인됩니다.", "verified": false})
		return
	case "/api/kdhc/query":
		if r.Method != "GET" {
			errorJSON(w, 405, errors.New("GET만 허용"))
			return
		}
		start, end := r.URL.Query().Get("startDate"), r.URL.Query().Get("endDate")
		st, e := parseKDHCDay(start)
		if e != nil {
			errorJSON(w, 400, e)
			return
		}
		ed, e := parseKDHCDay(end)
		if e != nil {
			errorJSON(w, 400, e)
			return
		}
		if st.After(ed) || ed.Sub(st) > 7*24*time.Hour {
			errorJSON(w, 400, errors.New("최대 7일 구간만 조회할 수 있습니다"))
			return
		}
		kdhcMemory.RLock()
		key, endpoint := kdhcMemory.key, kdhcMemory.endpoint
		kdhcMemory.RUnlock()
		if key == "" || endpoint == "" {
			errorJSON(w, 400, errors.New("인증키와 요청 URL을 먼저 등록하세요"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		result, e := queryKDHC(ctx, endpoint, key, start, end, 100)
		kdhcMemory.Lock()
		kdhcMemory.verified = e == nil
		kdhcMemory.Unlock()
		if e != nil {
			errorJSON(w, 502, e)
			return
		}
		sendJSON(w, 200, result)
		return
	}
	errorJSON(w, 404, errors.New("지원하지 않는 경로"))
}
