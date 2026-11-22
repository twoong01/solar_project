package main

// Fully paginated three-day KDHC inspection. It NEVER automatically converts
// plant aggregates into the original 28-inverter model inputs without proof.
import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
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

const kdhcPageSize = 100
const kdhcMaxPagesDay = 75

var lastKDHCBatch struct {
	sync.RWMutex
	csv []byte
}

type kdhcBatchSummary struct {
	Rows               int      `json:"rows"`
	Pages              []int    `json:"pages"`
	RowsByDay          []int    `json:"rowsByDay"`
	MatchedSites       int      `json:"matchedSites"`
	IncompleteSites    []string `json:"incompleteSites"`
	DuplicateNameHours int      `json:"duplicateNameHours"`
	UnknownNameRows    int      `json:"unknownNameRows"`
	ModelReady         bool     `json:"modelReady"`
	Note               string   `json:"note"`
}

func requestKDHCPage(ctx context.Context, endpoint, key, day string, page int) ([]map[string]string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.New("호출 URL 오류")
	}
	q := u.Query()
	q.Set("serviceKey", key)
	q.Set("startDate", day)
	q.Set("endDate", day)
	q.Set("numOfRows", strconv.Itoa(kdhcPageSize))
	q.Set("pageNo", strconv.Itoa(page))
	u.RawQuery = q.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("요청 생성 실패")
	}
	request.Header.Set("Accept", "application/xml,text/xml,*/*")
	response, err := kdhcHTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("날짜 %s / %d페이지 접속 실패: %s", day, page, cleanKDHCErr(err.Error(), key))
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, errors.New("API 결과 읽기 실패")
	}
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("날짜 %s / %d페이지 HTTP %d", day, page, response.StatusCode)
	}
	parsed, _, err := parseKDHCPayload(body)
	if err != nil {
		// Empty page is the normal termination for an exact page-size multiple.
		if page > 1 && strings.Contains(err.Error(), "항목(item)이 없습니다") {
			return []map[string]string{}, nil
		}
		return nil, fmt.Errorf("%s / %d페이지: %s", day, page, cleanKDHCErr(err.Error(), key))
	}
	return parsed.Items, nil
}

func collectKDHC(ctx context.Context, endpoint, key, asOf string) (kdhcBatchSummary, []byte, error) {
	var result kdhcBatchSummary
	end, err := time.Parse("2006-01-02", asOf)
	if err != nil || end.Format("2006-01-02") != asOf {
		return result, nil, errors.New("마지막 관측일은 YYYY-MM-DD 형식이어야 합니다")
	}
	if end.After(time.Now().AddDate(0, 0, 1)) {
		return result, nil, errors.New("미래 자료는 조회할 수 없습니다")
	}
	days := []string{end.AddDate(0, 0, -2).Format("20060102"), end.AddDate(0, 0, -1).Format("20060102"), end.Format("20060102")}
	var all []map[string]string
	result.Pages = make([]int, 0, 3)
	result.RowsByDay = make([]int, 0, 3)
	for _, day := range days {
		start := len(all)
		seenPageFirst := map[string]bool{}
		nPages := 0
		complete := false
		for page := 1; page <= kdhcMaxPagesDay; page++ {
			if err := ctx.Err(); err != nil {
				return result, nil, errors.New("전체 수집 시간 초과. 인터넷 상태를 확인하고 재시도하세요")
			}
			rows, err := requestKDHCPage(ctx, endpoint, key, day, page)
			if err != nil {
				return result, nil, err
			}
			nPages++
			if len(rows) > 0 {
				// Compare the full page: individual rows can repeat at night.
				signature, _ := json.Marshal(rows)
				if seenPageFirst[string(signature)] {
					return result, nil, fmt.Errorf("%s 페이지 %d: 동일한 페이지가 반복됩니다. API 페이지번호를 확인해야 합니다", day, page)
				}
				seenPageFirst[string(signature)] = true
			}
			all = append(all, rows...)
			if len(rows) < kdhcPageSize {
				complete = true
				break
			}
		}
		if !complete {
			return result, nil, fmt.Errorf("%s: 페이지 수가 상한을 초과했습니다. 부분 데이터는 사용하지 않습니다", day)
		}
		result.Pages = append(result.Pages, nPages)
		result.RowsByDay = append(result.RowsByDay, len(all)-start)
	}
	result.Rows = len(all)
	if len(all) == 0 {
		return result, nil, errors.New("3일치 API 관측 결과가 비어 있습니다")
	}
	csvBytes, err := writeRawKDHCCSV(all)
	if err != nil {
		return result, nil, err
	}
	result = inspectKDHCBatch(result, all, days)
	return result, csvBytes, nil
}

func writeRawKDHCCSV(rows []map[string]string) ([]byte, error) {
	fields := []string{"genName", "ymd", "workHr", "accumGen", "rnum"}
	known := map[string]bool{}
	for _, f := range fields {
		known[strings.ToLower(f)] = true
	}
	extra := map[string]bool{}
	for _, r := range rows {
		for k := range r {
			if !known[strings.ToLower(k)] {
				extra[k] = true
			}
		}
	}
	var extras []string
	for k := range extra {
		extras = append(extras, k)
	}
	sort.Strings(extras)
	fields = append(fields, extras...)
	var b bytes.Buffer
	b.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&b)
	if err := w.Write(fields); err != nil {
		return nil, err
	}
	for _, r := range rows {
		values := make([]string, len(fields))
		for i, f := range fields {
			values[i] = valueIgnoreCase(r, f)
		}
		if err := w.Write(values); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

func matchSite(name string) string {
	name = strings.TrimSpace(name)
	for _, s := range kdhcSites {
		if name == s || strings.HasPrefix(name, s+"(") || strings.HasPrefix(name, s+"（") {
			return s
		}
	}
	return ""
}
func inspectKDHCBatch(s kdhcBatchSummary, rows []map[string]string, days []string) kdhcBatchSummary {
	sitehours := map[string]map[string]bool{}
	duplicates := map[string]bool{}
	dayNames := map[string]bool{}
	for _, r := range rows {
		name := strings.TrimSpace(valueIgnoreCase(r, "genName"))
		day := strings.TrimSpace(valueIgnoreCase(r, "ymd"))
		hour := strings.TrimSpace(valueIgnoreCase(r, "workHr"))
		if len(hour) == 1 {
			hour = "0" + hour
		}
		site := matchSite(name)
		if site == "" {
			s.UnknownNameRows++
			continue
		}
		if _, err := strconv.Atoi(hour); err != nil || len(hour) != 2 {
			continue
		}
		h, err := strconv.Atoi(hour)
		if err != nil || h < 0 || h > 23 {
			continue
		}
		if !strings.Contains("|"+strings.Join(days, "|")+"|", "|"+day+"|") {
			continue
		}
		if sitehours[site] == nil {
			sitehours[site] = map[string]bool{}
		}
		if !dayNames[site] {
			dayNames[site] = true
			s.MatchedSites++
		}
		sitehours[site][day+":"+hour] = true
		key := name + "/" + day + "/" + hour
		if duplicates[key] {
			s.DuplicateNameHours++
		} else {
			duplicates[key] = true
		}
	}
	for _, site := range kdhcSites {
		if len(sitehours[site]) != 72 {
			s.IncompleteSites = append(s.IncompleteSites, fmt.Sprintf("%s %d/72", site, len(sitehours[site])))
		}
	}
	s.ModelReady = false
	s.Note = "전체 API 원본 수집 완료. 시간별 카운터를 모델 입력으로 자동 변환하지 않았습니다. genName만으로는 기존 28개 인버터 ID가 검증되지 않으며, accumGen은 누적 kWh로 모델 원자료 단위·리셋 기준과 별도 대조가 필요합니다. 모델 입력을 자동으로 채우거나 예측을 생성하지 않습니다."
	return s
}

func (a *app) handleKDHCCollect(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/kdhc/last.csv" {
		if r.Method != "GET" {
			errorJSON(w, 405, errors.New("GET만 허용"))
			return
		}
		lastKDHCBatch.RLock()
		data := append([]byte(nil), lastKDHCBatch.csv...)
		lastKDHCBatch.RUnlock()
		if len(data) == 0 {
			errorJSON(w, 404, errors.New("먼저 전체 데이터 수집을 실행하세요"))
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="kdhc_3days_raw.csv"`)
		_, _ = w.Write(data)
		return
	}
	if r.URL.Path != "/api/kdhc/collect" {
		errorJSON(w, 404, errors.New("지원하지 않는 경로"))
		return
	}
	if r.Method != "POST" {
		errorJSON(w, 405, errors.New("POST만 허용"))
		return
	}
	d, err := readBody(r, 2000)
	if err != nil {
		errorJSON(w, 400, err)
		return
	}
	asOf, _ := d["asOf"].(string)
	kdhcMemory.RLock()
	key, endpoint := kdhcMemory.key, kdhcMemory.endpoint
	kdhcMemory.RUnlock()
	if key == "" || endpoint == "" {
		errorJSON(w, 400, errors.New("먼저 API 키와 URL을 등록하세요"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	result, raw, err := collectKDHC(ctx, endpoint, key, asOf)
	if err != nil {
		errorJSON(w, 502, err)
		return
	}
	lastKDHCBatch.Lock()
	lastKDHCBatch.csv = raw
	lastKDHCBatch.Unlock()
	sendJSON(w, 200, result)
}
