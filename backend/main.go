// SolarView.exe: standalone Windows local dashboard + KPX solar-only API.
// Cross-compile: GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o SolarView.exe .
package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed dist/* data/kpx_solar_sample.csv data/kpx_solar_2025_all.json
var embedded embed.FS

const apiURL = "https://api.odcloud.kr/api/15065269/v1/uddi:2d675f57-b442-497b-a973-e35746c338b7"
const version = "10.0-react-kdhc"

var kpxAPIURL = apiURL // replaceable only in server-side tests

var regions = []string{"강원도", "경기도", "경상남도", "경상북도", "광주시", "대구시", "대전시", "부산시", "서울시", "세종시", "울산시", "인천시", "전라남도", "전라북도", "제주도", "충청남도", "충청북도"}
var needed = []string{"preprocessing.json", "metrics.json", "site_metrics.csv", "test_hourly_predictions.csv", "next_day_hourly_forecast.csv", "GRU/best.ckpt", "TFT/best.ckpt"}

type app struct {
	root     string
	key      string
	keyMu    sync.RWMutex
	verified bool
	id       string
	exit     chan struct{}
	savedMu  sync.Mutex
}
type payload map[string]any

func main() {
	root, err := executableFolder()
	if err != nil {
		log.Fatal(err)
	}
	a := &app{root: root, id: newID(), exit: make(chan struct{})}
	if k := os.Getenv("SOLAR_ODCLOUD_SERVICE_KEY"); k != "" {
		if dk, err := decodeKey(k); err == nil {
			a.key = dk
		}
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handle)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 75 * time.Second, MaxHeaderBytes: 1 << 20}
	u := fmt.Sprintf("http://127.0.0.1:%d/?solarview=offline-v7", port)
	fmt.Printf("\n Solar View Offline Data (%s)\n Website: %s\n API test: http://127.0.0.1:%d/api/health\n Close this window or press Ctrl+C to stop.\n\n", version, u, port)
	// No fixed 8765 port: each run uses a port atomically allocated by Windows.
	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Println("Server:", err)
		}
	}()
	time.Sleep(180 * time.Millisecond)
	if err := openBrowser(u); err != nil {
		fmt.Printf("Open this URL in Chrome or Edge: %s\n", u)
	}
	<-a.exit
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
func executableFolder() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(p), nil
}
func newID() string { b := make([]byte, 8); _, _ = rand.Read(b); return fmt.Sprintf("%x", b) }
func openBrowser(u string) error {
	if runtime.GOOS == "windows" {
		return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", u).Start()
	}
	if runtime.GOOS == "darwin" {
		return exec.Command("open", u).Start()
	}
	return exec.Command("xdg-open", u).Start()
}
func sendJSON(w http.ResponseWriter, status int, p any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}
func errorJSON(w http.ResponseWriter, status int, err error) {
	sendJSON(w, status, payload{"ok": false, "error": err.Error()})
}
func (a *app) guard(r *http.Request) bool {
	h := r.Host
	host, _, err := net.SplitHostPort(h)
	if err != nil {
		host = h
	}
	if host != "127.0.0.1" && host != "localhost" && host != "[::1]" && host != "::1" {
		return false
	}
	if r.Method != "POST" {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	o, err := url.Parse(origin)
	return err == nil && o.Scheme == "http" && (o.Hostname() == "127.0.0.1" || o.Hostname() == "localhost")
}
func (a *app) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	if !a.guard(r) {
		errorJSON(w, 403, errors.New("로컬 브라우저에서만 접근할 수 있습니다."))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		a.handleAPI(w, r)
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		errorJSON(w, 405, errors.New("지원하지 않는 요청"))
		return
	}
	// React/Vite production assets are embedded at build time; allow safe nested /assets/* paths.
	requestPath := r.URL.Path
	if requestPath == "/" || requestPath == "/index" || requestPath == "/home" || requestPath == "/dashboard" || requestPath == "/forecast" || requestPath == "/app" {
		requestPath = "/index.html"
	}
	clean := filepath.ToSlash(filepath.Clean(requestPath))
	if clean != requestPath || strings.Contains(requestPath, "\\") || strings.Contains(requestPath, "..") {
		http.NotFound(w, r)
		return
	}
	if !strings.Contains(filepath.Base(requestPath), ".") {
		requestPath = "/index.html"
	}
	ext := filepath.Ext(requestPath)
	allowedExt := map[string]bool{".html": true, ".js": true, ".css": true, ".json": true, ".png": true, ".jpg": true, ".svg": true, ".ico": true, ".woff": true, ".woff2": true, ".webp": true, ".map": true}
	if !allowedExt[ext] {
		http.NotFound(w, r)
		return
	}
	mimeType := mime.TypeByExtension(ext)
	if ext == ".js" {
		mimeType = "text/javascript; charset=utf-8"
	}
	if ext == ".json" {
		mimeType = "application/json; charset=utf-8"
	}
	if mimeType == "" {
		mimeType = http.DetectContentType([]byte(""))
	}
	// data.json can be updated on disk after an inference run.
	var body []byte
	var readErr error
	if requestPath == "/data.json" {
		body, readErr = os.ReadFile(filepath.Join(a.root, "dist", "data.json"))
	}
	if body == nil || readErr != nil {
		body, readErr = embedded.ReadFile("dist" + requestPath)
	}
	if readErr != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		return
	}
	_, _ = w.Write(body)
}
func (a *app) handleAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasPrefix(path, "/api/kdhc/") {
		a.handleKDHC(w, r)
		return
	}
	if r.Method == "GET" {
		switch path {
		case "/api/health":
			sendJSON(w, 200, payload{"app": "solarview", "version": version, "instanceId": a.id, "ready": true, "home": "/index.html", "port": r.Host})
			return
		case "/api/kpx/status":
			a.keyMu.RLock()
			configured := a.key != ""
			verified := a.verified
			a.keyMu.RUnlock()
			sendJSON(w, 200, payload{"configured": configured, "verified": verified, "endpoint": apiURL, "source": "2025년 태양광 전체 CSV 내장 / 과거 거래량 (실시간 아님)", "regions": regions, "offlineAvailable": true, "offlineStart": "2025-01-01", "offlineEnd": "2025-12-31", "forecastModelInputCompatible": false})
			return
		case "/api/kpx/diagnose":
			a.keyMu.RLock()
			k := a.key
			a.keyMu.RUnlock()
			if k == "" {
				errorJSON(w, 400, errors.New("API 인증키를 먼저 등록해 주세요."))
				return
			}
			info, err := diagnoseKPX(k)
			if err != nil {
				a.keyMu.Lock()
				a.verified = false
				a.keyMu.Unlock()
				errorJSON(w, 502, err)
				return
			}
			a.keyMu.Lock()
			a.verified = true
			a.keyMu.Unlock()
			sendJSON(w, 200, info)
			return
		case "/api/kpx/day", "/api/kpx/sample", "/api/kpx/local":
			day := r.URL.Query().Get("date")
			if day == "" {
				day = "2025-12-31"
			}
			region := r.URL.Query().Get("region")
			if region == "" {
				region = "all"
			}
			if err := validateQuery(day, region); err != nil {
				errorJSON(w, 400, err)
				return
			}
			var out any
			var err error
			if path == "/api/kpx/local" {
				out, err = loadOfflineSolar(day, region)
			} else if path == "/api/kpx/sample" {
				out, err = loadSample(day, region)
			} else {
				a.keyMu.RLock()
				k := a.key
				a.keyMu.RUnlock()
				if k == "" {
					errorJSON(w, 400, errors.New("API 키를 먼저 등록해 주세요."))
					return
				}
				out, err = fetchKPX(day, region, k)
				if err == nil {
					a.keyMu.Lock()
					a.verified = true
					a.keyMu.Unlock()
				}
			}
			if err != nil {
				errorJSON(w, 400, err)
				return
			}
			sendJSON(w, 200, out)
			return
		case "/api/status":
			s := a.sourceFolder()
			missing := []string{}
			for _, name := range needed {
				if _, err := os.Stat(filepath.Join(s, filepath.FromSlash(name))); err != nil {
					missing = append(missing, name)
				}
			}
			ready := len(missing) == 0
			var source any = nil
			if ready {
				source = s
			}
			sendJSON(w, 200, payload{"ready": ready, "source": source, "missing": missing, "suggestedLocations": []string{s}})
			return
		}
		errorJSON(w, 404, errors.New("지원하지 않는 API 경로"))
		return
	}
	if r.Method != "POST" {
		errorJSON(w, 405, errors.New("POST 요청만 지원합니다."))
		return
	}
	switch path {
	case "/api/kpx/auth":
		d, err := readBody(r, 10000)
		if err != nil {
			errorJSON(w, 400, err)
			return
		}
		raw, ok := d["key"].(string)
		if !ok {
			errorJSON(w, 400, errors.New("인증키를 입력하세요."))
			return
		}
		key, err := decodeKey(raw)
		if err != nil {
			errorJSON(w, 400, err)
			return
		}
		a.keyMu.Lock()
		a.key = key
		a.verified = false // storing a string is not authentication
		a.keyMu.Unlock()
		sendJSON(w, 200, payload{"ok": true, "configured": true, "message": "인증키를 저장했습니다. 실제 인증 여부는 API 연결 진단으로 확인하세요."})
		return
	case "/api/data":
		d, err := readBody(r, 25<<20)
		if err != nil {
			errorJSON(w, 400, err)
			return
		}
		if err = validateDashboard(d); err != nil {
			errorJSON(w, 400, err)
			return
		}
		if err = a.saveDashboard(d); err != nil {
			errorJSON(w, 400, err)
			return
		}
		sendJSON(w, 200, payload{"ok": true, "message": "변경된 데이터를 저장했습니다."})
		return
	case "/api/refresh":
		_, err := readBody(r, 10000)
		if err != nil {
			errorJSON(w, 400, err)
			return
		}
		err = a.runPythonScript("export_data.py", 90*time.Second)
		if err != nil {
			errorJSON(w, 400, err)
			return
		}
		sendJSON(w, 200, payload{"ok": true, "message": "원본 저장 결과를 다시 불러왔습니다."})
		return
	case "/api/predict":
		d, err := readBody(r, 16<<20)
		if err != nil {
			errorJSON(w, 400, err)
			return
		}
		asof, _ := d["asOf"].(string)
		csv, _ := d["csv"].(string)
		if _, err = validateDate(asof); err != nil {
			errorJSON(w, 400, err)
			return
		}
		result, err := a.predict(asof, csv)
		if err != nil {
			errorJSON(w, 400, err)
			return
		}
		sendJSON(w, 200, result)
		return
	case "/api/exit":
		_, err := readBody(r, 1000)
		if err != nil {
			errorJSON(w, 400, err)
			return
		}
		sendJSON(w, 200, payload{"ok": true})
		go func() {
			time.Sleep(150 * time.Millisecond)
			select {
			case a.exit <- struct{}{}:
			default:
			}
		}()
		return
	}
	errorJSON(w, 404, errors.New("지원하지 않는 요청"))
}
func readBody(r *http.Request, limit int64) (payload, error) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return nil, errors.New("JSON 요청이 필요합니다.")
	}
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(b) > int(limit) {
		return nil, errors.New("파일 크기 제한 초과")
	}
	var d payload
	dec := json.NewDecoder(strings.NewReader(string(b)))
	if err = dec.Decode(&d); err != nil {
		return nil, errors.New("JSON 내용이 올바르지 않습니다.")
	}
	if d == nil {
		return nil, errors.New("JSON 객체가 필요합니다.")
	}
	return d, nil
}
func validateDashboard(d payload) error {
	sites, ok := d["sites"].([]any)
	if !ok || len(sites) < 1 || len(sites) > 200 {
		return errors.New("발전소 목록이 없습니다.")
	}
	tests, ok := d["test"].(map[string]any)
	if !ok || len(tests) == 0 {
		return errors.New("검증 날짜가 없습니다.")
	}
	forecast, ok := d["forecast"].(map[string]any)
	if !ok {
		return errors.New("예측 데이터가 없습니다.")
	}
	fdate, _ := d["forecastDate"].(string)
	if _, err := validateDate(fdate); err != nil {
		return err
	}
	if _, ok := d["sourceEnd"].(string); !ok {
		return errors.New("관측 종료일이 없습니다.")
	}
	if _, ok := d["inverters"].(map[string]any); !ok {
		return errors.New("인버터 목록이 없습니다.")
	}
	if _, ok := d["metrics"].(map[string]any); !ok {
		return errors.New("성능 지표가 없습니다.")
	}
	if _, ok := d["siteMetrics"].([]any); !ok {
		return errors.New("지역별 성능 지표가 없습니다.")
	}
	for date, rows := range tests {
		if _, err := validateDate(date); err != nil {
			return err
		}
		if err := checkRows(sites, rows); err != nil {
			return err
		}
	}
	return checkRows(sites, forecast)
}
func checkRows(sites []any, data any) error {
	group, ok := data.(map[string]any)
	if !ok {
		return errors.New("지역별 시간 데이터가 없습니다.")
	}
	for _, raw := range sites {
		site, ok := raw.(string)
		if !ok || len(site) > 120 {
			return errors.New("지역명 오류")
		}
		hours, ok := group[site].([]any)
		if !ok || len(hours) != 24 {
			return fmt.Errorf("%s: 24시간 데이터가 필요합니다", site)
		}
		for _, h := range hours {
			arr, ok := h.([]any)
			if !ok || len(arr) != 5 {
				return errors.New("발전량 데이터는 시간별 5열이어야 합니다.")
			}
			for _, v := range arr {
				if v == nil {
					continue
				}
				n, ok := v.(float64)
				if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
					return errors.New("발전량 숫자 오류")
				}
			}
		}
	}
	return nil
}
func (a *app) saveDashboard(d payload) error {
	a.savedMu.Lock()
	defer a.savedMu.Unlock()
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	dir := filepath.Join(a.root, "dist")
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for name, contents := range map[string][]byte{"data.json": b, "data.js": append([]byte("window.__SOLAR_DATA__ = "), append(b, []byte(";\n")...)...)} {
		tmp := filepath.Join(dir, "."+name+".tmp")
		if err = os.WriteFile(tmp, contents, 0644); err != nil {
			return err
		}
		if err = os.Rename(tmp, filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}
func (a *app) sourceFolder() string {
	if d := os.Getenv("SOLAR_ARTIFACTS_DIR"); d != "" {
		return d
	}
	return filepath.Join(a.root, "model", "artifacts")
}
func (a *app) python() (string, []string, error) {
	var options = []struct {
		bin  string
		args []string
	}{{filepath.Join(a.root, ".venv", "Scripts", "python.exe"), nil}, {"py", []string{"-3.12"}}, {"python", nil}, {"python3", nil}}
	for _, opt := range options {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		args := append(append([]string{}, opt.args...), "-c", "import sys; sys.exit(0 if sys.version_info.major == 3 and sys.version_info.minor == 12 else 1)")
		cmd := exec.CommandContext(ctx, opt.bin, args...)
		cmd.Dir = a.root
		err := cmd.Run()
		cancel()
		if err == nil {
			return opt.bin, opt.args, nil
		}
	}
	return "", nil, errors.New("실제 GRU/TFT 추론에는 Python 3.12 + 모델 라이브러리가 필요합니다. 웹 화면과 공식 API 조회는 Python 없이 작동합니다.")
}
func (a *app) runPythonScript(script string, timeout time.Duration) error {
	py, args, err := a.python()
	if err != nil {
		return err
	}
	scriptPath := filepath.Join(a.root, script)
	if _, err := os.Stat(scriptPath); err != nil {
		return errors.New("원본 Python 코드가 EXE 폴더에 없습니다.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	all := append(append([]string{}, args...), scriptPath)
	cmd := exec.CommandContext(ctx, py, all...)
	cmd.Dir = a.root
	b, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return errors.New("원본 코드 실행 시간 초과")
		}
		txt := string(b)
		if len(txt) > 700 {
			txt = txt[len(txt)-700:]
		}
		return fmt.Errorf("원본 코드 실행 실패: %s", txt)
	}
	return nil
}
func (a *app) predict(asof, csvText string) (payload, error) {
	py, pre, err := a.python()
	if err != nil {
		return nil, err
	}
	// The original Python model_runner validates every region/hour and invokes original checkpoints.
	bridge := filepath.Join(a.root, "go_model_bridge.py")
	if _, err := os.Stat(bridge); err != nil {
		return nil, errors.New("모델 추론 연결 코드(go_model_bridge.py)가 누락되었습니다.")
	}
	req := payload{"asOf": asof}
	if csvText != "" {
		req["csv"] = csvText
	}
	tmp, err := os.CreateTemp("", "solarview-request-*.json")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err = json.NewEncoder(tmp).Encode(req); err != nil {
		return nil, err
	}
	_ = tmp.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	args := append(append([]string{}, pre...), bridge, tmp.Name())
	cmd := exec.CommandContext(ctx, py, args...)
	cmd.Dir = a.root
	b, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("추론 시간이 초과되었습니다.")
		}
		msg := string(b)
		if len(msg) > 900 {
			msg = msg[len(msg)-900:]
		}
		return nil, fmt.Errorf("원본 모델 실행 오류: %s", msg)
	}
	var result payload
	if err = json.Unmarshal(b, &result); err != nil {
		return nil, errors.New("원본 모델 결과를 읽을 수 없습니다.")
	}
	return result, nil
}
func validateDate(day string) (time.Time, error) {
	d, err := time.Parse("2006-01-02", day)
	if err != nil || d.Format("2006-01-02") != day {
		return time.Time{}, errors.New("날짜는 YYYY-MM-DD 형식이어야 합니다.")
	}
	return d, nil
}
func validateQuery(date, region string) error {
	if _, err := validateDate(date); err != nil {
		return err
	}
	if region != "all" {
		found := false
		for _, r := range regions {
			if r == region {
				found = true
				break
			}
		}
		if !found {
			return errors.New("지원하지 않는 시·도입니다.")
		}
	}
	return nil
}
func decodeKey(s string) (string, error) {
	s = strings.TrimSpace(s)
	if len(s) < 12 || len(s) > 600 {
		return "", errors.New("인증키 형식이 올바르지 않습니다.")
	}
	if strings.Contains(s, "%") {
		var err error
		s, err = url.QueryUnescape(s)
		if err != nil {
			return "", errors.New("인증키 디코딩 실패")
		}
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return "", errors.New("인증키에 허용되지 않는 문자가 있습니다.")
	}
	return s, nil
}

type rawRow map[string]string
type cleanRow struct {
	Region string
	Hour   int
	MWh    float64
}

func normalizeRow(r rawRow, day, region string) (cleanRow, bool) {
	if r["연료원"] != "태양광" {
		return cleanRow{}, false
	}
	d := strings.ReplaceAll(strings.TrimSpace(r["거래일"]), ".", "-")
	if len(d) == 8 && !strings.Contains(d, "-") {
		d = d[:4] + "-" + d[4:6] + "-" + d[6:]
	}
	name := strings.TrimSpace(r["지역"])
	if d != day || (region != "all" && name != region) {
		return cleanRow{}, false
	}
	hour, err := strconv.Atoi(strings.TrimSpace(r["거래시간"]))
	if err != nil || hour < 1 || hour > 24 {
		return cleanRow{}, false
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(r["전력거래량(MWh)"], ",", ""), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n >= 1e12 {
		return cleanRow{}, false
	}
	return cleanRow{name, hour, n}, true
}
func summary(rows []rawRow, day, region, source string) (payload, error) {
	buckets := make([]float64, 24)
	names := map[string]bool{}
	seen := map[string]bool{}
	count := 0
	for _, r := range rows {
		row, ok := normalizeRow(r, day, region)
		if !ok {
			continue
		}
		identity := row.Region + ":" + strconv.Itoa(row.Hour)
		if seen[identity] {
			return nil, errors.New("API 지역·시간 중복 행이 있습니다.")
		}
		seen[identity] = true
		names[row.Region] = true
		buckets[row.Hour-1] += row.MWh
		count++
	}
	intervals := make([]payload, 24)
	total := 0.
	for i, n := range buckets {
		intervals[i] = payload{"hour": i + 1, "mwh": math.Round(n*1e6) / 1e6}
		total += n
	}
	namesList := make([]string, 0, len(names))
	for n := range names {
		namesList = append(namesList, n)
	}
	sort.Strings(namesList)
	note := "전력거래소 시·도별 거래량: 연간 갱신 데이터이며, 기존 13개 발전소 모델의 실시간 입력 자료가 아닙니다."
	if len(namesList) > 0 && len(seen) != 24*len(namesList) {
		note += " 해당 날짜의 일부 시각 데이터가 누락됐습니다."
	}
	return payload{"date": day, "region": region, "source": source, "isRealtime": false, "updatedAt": time.Now().Format(time.RFC3339), "sourceLastPublished": "2025-12-31", "unit": "MWh", "fuel": "태양광", "intervals": intervals, "totalMWh": math.Round(total*1e6) / 1e6, "records": count, "regions": namesList, "complete": len(namesList) > 0 && len(seen) == 24*len(namesList), "note": note}, nil
}

// Embedded solar-only 2025 dataset: 365 days x 17 regions x 24 hourly trades.
// Does not require API credentials, Python, internet or external CSV path.
type offlineSolar struct {
	PeriodStart string                 `json:"periodStart"`
	PeriodEnd   string                 `json:"periodEnd"`
	Regions     []string               `json:"regions"`
	Days        map[string][][]float64 `json:"days"`
}

var offlineOnce sync.Once
var offlineStore offlineSolar
var offlineLoadError error

func offlineData() (*offlineSolar, error) {
	offlineOnce.Do(func() {
		b, err := embedded.ReadFile("data/kpx_solar_2025_all.json")
		if err != nil {
			offlineLoadError = err
			return
		}
		err = json.Unmarshal(b, &offlineStore)
		if err != nil {
			offlineLoadError = err
			return
		}
		if len(offlineStore.Regions) != 17 || len(offlineStore.Days) != 365 {
			offlineLoadError = errors.New("내장 CSV 데이터가 불완전합니다")
		}
	})
	if offlineLoadError != nil {
		return nil, offlineLoadError
	}
	return &offlineStore, nil
}
func loadOfflineSolar(day, region string) (payload, error) {
	if err := validateQuery(day, region); err != nil {
		return nil, err
	}
	store, err := offlineData()
	if err != nil {
		return nil, err
	}
	if day < store.PeriodStart || day > store.PeriodEnd {
		return nil, fmt.Errorf("내장 CSV는 2025-01-01~2025-12-31 기간만 포함합니다. 선택: %s", day)
	}
	values, exists := store.Days[day]
	if !exists {
		return nil, errors.New("해당 날짜의 태양광 데이터가 없습니다")
	}
	if len(values) != len(store.Regions) {
		return nil, errors.New("지역 데이터의 크기가 올바르지 않습니다")
	}
	totals := make([]float64, 24)
	names := []string{}
	count := 0
	for i, name := range store.Regions {
		if region != "all" && region != name {
			continue
		}
		if len(values[i]) != 24 {
			return nil, fmt.Errorf("%s의 24시간 자료가 불완전합니다", name)
		}
		names = append(names, name)
		for h, v := range values[i] {
			if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, errors.New("발전량 값에 오류가 있습니다")
			}
			totals[h] += v
			count++
		}
	}
	if len(names) == 0 {
		return nil, errors.New("선택 지역의 데이터가 없습니다")
	}
	result := make([]payload, 24)
	total := 0.0
	for h, v := range totals {
		result[h] = payload{"hour": h + 1, "mwh": math.Round(v*1e6) / 1e6}
		total += v
	}
	return payload{
		"date": day, "region": region, "source": "사용자 제공 2025 태양광 CSV · 전체 내장본",
		"isRealtime": false, "updatedAt": time.Now().Format(time.RFC3339),
		"sourceLastPublished": "2025-12-31", "fuel": "태양광", "unit": "MWh",
		"intervals": result, "totalMWh": math.Round(total*1e6) / 1e6,
		"records": count, "regions": names, "complete": count == 24*len(names),
		"note": "2025년 전력거래소 시·도별 태양광 전력거래량(MWh) 원본 CSV. 전체 365일·17개 시·도·24개 거래시간. 풍력 제외. 실시간 데이터가 아니며 기존 13개 발전소 GRU/TFT 모델 입력으로 사용되지 않습니다.",
	}, nil
}

func loadSample(day, region string) (payload, error) {
	b, err := embedded.ReadFile("data/kpx_solar_sample.csv")
	if err != nil {
		return nil, err
	}
	rows, err := parseCSV(b)
	if err != nil {
		return nil, err
	}
	return summary(rows, day, region, "사용자 제공 CSV 오프라인 샘플")
}
func parseCSV(b []byte) ([]rawRow, error) {
	b = []byte(strings.TrimPrefix(string(b), "\ufeff"))
	reader := csv.NewReader(strings.NewReader(string(b)))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("CSV 내용이 비어 있습니다.")
	}
	headers := records[0]
	rows := make([]rawRow, 0, len(records)-1)
	for _, record := range records[1:] {
		row := rawRow{}
		for i, key := range headers {
			if i < len(record) {
				row[strings.TrimSpace(key)] = record[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// providerError extracts provider diagnostics instead of hiding them as "malformed JSON".
// Only known error fields are returned, never headers, keys or raw response text.
func providerError(b []byte, key string) string {
	var obj map[string]json.RawMessage
	if json.Unmarshal(b, &obj) != nil {
		return "공식 API가 JSON이 아닌 응답을 반환했습니다. 공식 활용명세를 확인하세요."
	}
	msg := ""
	for _, field := range []string{"msg", "message", "detail", "error_description", "returnAuthMsg", "error", "errorMessage", "code"} {
		raw, found := obj[field]
		if !found {
			continue
		}
		var val any
		if json.Unmarshal(raw, &val) != nil {
			continue
		}
		switch x := val.(type) {
		case string:
			if strings.TrimSpace(x) != "" {
				msg += field + "=" + x + "; "
			}
		case float64:
			msg += fmt.Sprintf("%s=%v; ", field, x)
		}
	}
	if msg == "" {
		msg = "공식 API가 data 배열을 반환하지 않았습니다. 인증키·활용신청·조회조건을 확인하세요."
	}
	if key != "" {
		msg = strings.ReplaceAll(msg, key, "[숨김]")
	}
	if len([]rune(msg)) > 240 {
		msg = string([]rune(msg)[:240]) + "…"
	}
	return msg
}

type responseKPX struct {
	data       []map[string]any
	matchCount int
}

// requestKPX requests one page of the selected public API. Credentials stay in a header.
func requestKPX(client *http.Client, endpoint string, q url.Values, key string) (responseKPX, error) {
	base, err := url.Parse(endpoint)
	if err != nil {
		return responseKPX{}, errors.New("API URL 구성이 올바르지 않습니다.")
	}
	base.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodGet, base.String(), nil)
	if err != nil {
		return responseKPX{}, err
	}
	req.Header.Set("Authorization", "Infuser "+key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "SolarView-Windows-EXE/6.0")
	resp, err := client.Do(req)
	if err != nil {
		return responseKPX{}, fmt.Errorf("공식 API 서버 연결 실패: %v", err)
	}
	limited, readErr := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	_ = resp.Body.Close()
	if readErr != nil {
		return responseKPX{}, errors.New("공식 API 응답 읽기 실패")
	}
	reason := providerError(limited, key)
	if resp.StatusCode != 200 {
		if resp.StatusCode == 401 {
			return responseKPX{}, fmt.Errorf("공식 API HTTP 401 (인증키 확인 필요): %s", reason)
		}
		if resp.StatusCode == 403 {
			return responseKPX{}, fmt.Errorf("공식 API HTTP 403 (활용신청·승인·권한 확인 필요): %s", reason)
		}
		if resp.StatusCode == 429 {
			return responseKPX{}, fmt.Errorf("공식 API HTTP 429 (호출 한도 초과): %s", reason)
		}
		return responseKPX{}, fmt.Errorf("공식 API HTTP %d: %s", resp.StatusCode, reason)
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(limited, &obj) != nil {
		return responseKPX{}, errors.New(reason)
	}
	dataRaw, ok := obj["data"]
	if !ok || string(dataRaw) == "null" {
		return responseKPX{}, errors.New(reason)
	}
	var data []map[string]any
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return responseKPX{}, errors.New("공식 API 데이터 배열의 구조가 예상과 다릅니다: " + reason)
	}
	count := -1
	if raw, ok := obj["matchCount"]; ok {
		var n any
		if json.Unmarshal(raw, &n) == nil {
			switch t := n.(type) {
			case float64:
				count = int(t)
			case string:
				if v, e := strconv.Atoi(t); e == nil {
					count = v
				}
			}
		}
	}
	return responseKPX{data: data, matchCount: count}, nil
}

// diagnoseKPX verifies actual remote authentication with a minimal one-row request.
// This does not claim data is live or the 13-site model can use these values.
func diagnoseKPX(key string) (payload, error) {
	client := &http.Client{Timeout: 18 * time.Second}
	q := url.Values{"page": {"1"}, "perPage": {"1"}, "returnType": {"json"}}
	resp, err := requestKPX(client, kpxAPIURL, q, key)
	if err != nil {
		return nil, fmt.Errorf("API 연결 진단 실패: %w", err)
	}
	keys := []string{}
	exampleDate := ""
	if len(resp.data) > 0 {
		for k := range resp.data[0] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if v, ok := resp.data[0]["거래일"]; ok {
			exampleDate = fmt.Sprint(v)
		}
	}
	return payload{"ok": true, "authenticated": true, "sampleFields": keys,
		"exampleDate": exampleDate, "firstPageRows": len(resp.data),
		"message": "공식 API 응답을 정상 수신했습니다. 단, 날짜별 자료가 존재하는지는 별도로 조회해야 합니다."}, nil
}

func fetchKPX(day, region, key string) (payload, error) {
	client := &http.Client{Timeout: 22 * time.Second}
	q := url.Values{"page": {"1"}, "perPage": {"500"}, "returnType": {"json"}, "cond[연료원::EQ]": {"태양광"}, "cond[거래일::EQ]": {day}}
	if region != "all" {
		q.Set("cond[지역::EQ]", region)
	}
	rows := []rawRow{}
	count := -1
	for page := 1; page <= 11; page++ {
		q.Set("page", strconv.Itoa(page))
		result, err := requestKPX(client, kpxAPIURL, q, key)
		if err != nil {
			// A bare one-row probe distinguishes bad filters from credentials/provider.
			probeQ := url.Values{"page": {"1"}, "perPage": {"1"}, "returnType": {"json"}}
			_, probeErr := requestKPX(client, kpxAPIURL, probeQ, key)
			if probeErr == nil {
				return nil, fmt.Errorf("인증키는 공식 API에서 응답하지만 이 조회 조건은 실패했습니다. 거래일 %s·지역 %s·태양광 필터 확인 필요. API 메시지: %w", day, region, err)
			}
			return nil, fmt.Errorf("조회 실패. 최소 조회 진단도 실패했습니다: %w", probeErr)
		}
		for _, r := range result.data {
			rec := rawRow{}
			for k, v := range r {
				rec[k] = fmt.Sprint(v)
			}
			rows = append(rows, rec)
		}
		if result.matchCount >= 0 {
			count = result.matchCount
		}
		if len(result.data) < 500 || (count >= 0 && page*500 >= count) {
			break
		}
		if page == 11 {
			return nil, errors.New("조회 범위가 너무 넓습니다. 시·도와 날짜를 좁혀 주세요.")
		}
	}
	out, err := summary(rows, day, region, "공공데이터포털 odcloud API")
	if err != nil {
		return nil, err
	}
	if out["records"].(int) == 0 {
		return nil, fmt.Errorf("%s / %s의 태양광 결과가 없습니다. 조회 날짜를 2025-12-31로 바꾸어 확인해 보세요. API 공개 데이터가 실시간 제공되는 것은 아닙니다.", day, region)
	}
	return out, nil
}

// Ensure fs embed is retained in code path; Go's linker can strip unused data if not referenced.
var _ fs.FS = embedded
