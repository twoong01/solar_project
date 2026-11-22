# Solar View — React/Vite 리팩터링

> 기준: 업로드한 `SolarView_Windows_Offline_v7(1).zip`의 디자인·색상·레이아웃·데이터. 기존 프로젝트에서 개발하던 **한국지역난방공사 13개 발전소 API 진단 메뉴**도 독립 React 화면으로 추가했습니다.

## 1. 실행 (React 프론트엔드)

Node.js **18 이상**과 인터넷 연결(최초 `npm install` 시)이 필요합니다. Windows PowerShell / macOS 터미널에서 공통으로 실행합니다.

```bash
npm install
npm run dev
```

브라우저에서 터미널에 표시된 `http://127.0.0.1:5173`(또는 다른 포트)로 접속합니다. 원본 EXE를 실행할 필요는 없습니다.

- **발전량 예측 / 모델 검증:** 내장 `forecast.json`으로 작동. GRU·TFT 선택, 91개 저장 날짜, 지역, 시간별 그래프/테이블, CSV 내보내기
- **태양광 발전 실적 · CSV:** 2025년 전국 17개 시·도 태양광 전력거래량(365일, 148,920행)을 로컬 JSON에서 조회. 인증키/인터넷 연결 불필요 (최초 설치 후)
- **데이터 · 모델 정보:** 원본 모델과 성능 지표 표시
- **데이터 관리 · 갱신:** JSON 불러오기/기본 JSON 다시 불러오기. 실제 모델 재추론은 아래 별도 백엔드가 있어야 함
- **13개 발전소 API 연결:** 원본 KDHC 실측 API 테스트, 72시간 전체 페이지 수집·진단. **Go 서버 필요**, 인증키는 ZIP에 포함하지 않음

## 2. 폴더 구조

```text
SolarView_React_v1/
├── index.html                 # React root
├── package.json               # React 18, Vite 5
├── vite.config.js             # 개발 서버 / API 프록시
├── public/
│   ├── favicon.ico
│   └── data/
│       ├── forecast.json      # 기존 13개 발전소 GRU/TFT 저장 예측
│       └── kpx-solar-2025.json# 2025 시도별 태양광 CSV를 압축한 JSON
├── src/
│   ├── main.jsx               # React 진입점
│   ├── App.jsx                # 페이지 전환·공통 상태
│   ├── components/
│   │   ├── Sidebar.jsx, TopBar.jsx, PageHeading.jsx
│   │   ├── ForecastFilters.jsx, MetricCards.jsx
│   │   ├── Insights.jsx, PlantTable.jsx
│   │   ├── charts/            # ForecastChart, SolarBarChart
│   │   └── dialogs/           # 정보/데이터 관리 모달
│   ├── pages/                 # ForecastPage, KpxPage, KdhcPage
│   ├── services/              # JSON/API fetch 래퍼
│   ├── utils/                 # WAPE·집계·CSV 저장 등 순수 함수
│   └── styles/                # v7 CSS 원본 + 최소 React 보정
├── backend/                   # 선택 사항: Go API / Python 모델 실행기
│   ├── main.go, kdhc*.go       # 로컬 HTTP 서버·KPX/KDHC API
│   ├── model/                 # 기존 체크포인트·학습/예측 파일 포함
│   ├── data/                  # 오프라인 태양광 데이터
│   └── dist/                  # npm build 후 React 배포 파일 복사
├── scripts/                   # React → Go 배포 동기화 스크립트
└── tests/                     # 오프라인 데이터 무결성 검사
```

## 3. React 빌드

```bash
npm run build           # dist/ 생성
npm run preview         # 배포 정적 화면 확인
npm run test:data       # 13개 지역 / CSV 데이터 계산 검사
```

> `npm run build`는 **브라우저용 정적 화면**을 만들며, 이 자체에는 Python/Go 서버가 포함되지 않습니다. 정적 버전만으로 KPX 2025 CSV 및 저장 예측 조회는 됩니다.

## 4. Go API/실제 GRU·TFT 실행 기능과 연동

Go 1.21 이상 설치 후 `build:desktop` 실행 시 React 빌드 결과를 Go 실행 파일에 포함합니다.

```bash
npm run build:desktop
```

완료 후 **`backend/SolarView.exe`**가 생성됩니다 (Windows 64비트용). Mac에서 Mac용 바이너리를 빌드하려면:

```bash
TARGET_OS=darwin TARGET_ARCH=arm64 npm run build:desktop
```

빌드된 파일은 반드시 `backend/` 폴더에서 실행하고, 원본 `backend/model/`, `backend/dist/`, `backend/export_data.py`, `backend/model_runner.py`를 유지해야 Python 재추론과 업데이트가 가능합니다. 실제 모델 재추론은 Python 3.12와 `backend/model/requirements.txt`의 패키지 설치가 추가로 필요합니다.

### React 개발 서버에서 Go API 프록시 사용

Go 서버를 별도로 실행해 터미널에 표시되는 포트를 확인한 뒤:

```bash
# .env.local 에 다음처럼 작성한 후 npm run dev
VITE_API_PROXY=http://127.0.0.1:실제포트
```

해당 서버의 `/api/*`와 `/data.json`이 React 개발 서버로 프록시됩니다. **API 키를 VITE_* 설정 파일에 넣지 마세요.** 공개 번들에 포함될 수 있습니다. 키는 앱 화면에서 입력하고 Go 서버 메모리에서만 관리하도록 구현돼 있습니다.

## 5. 데이터 혼합 금지 / 최신성

| 데이터 | 역할 | 현재 범위 |
|---|---|---|
| 한국지역난방공사 원본 GRU/TFT 저장 예측 | 13개 지역 / 28개 인버터 모델 예측 화면 | 2026-01~03 테스트, 2026-04-01 예측 |
| 한국전력거래소 KPX CSV | 전국 17개 시도별 태양광 거래량(MWh) 현황 | 2025-01-01~2025-12-31 |
| 한국지역난방공사 실측 API | 원본 발전소/인버터·누적발전량 검사 | 서버가 받은 실제 응답에 따름 |

**KPX 17개 시도 실적을 GRU/TFT 13개 발전소의 입력처럼 자동 변환하지 않습니다.** KDHC API 결과도 인버터 식별·단위/차분 기준과 시간 연속성 검증 전에는 모델에 자동 투입하지 않습니다. 현재 소스는 2026년 11월의 실시간 예측 완성본이 아닙니다.

## 6. 검증 현황

- `npm run test:data`: 통과
- Go `go test ./...` (backend 폴더): 통과
- JSX/JS 정적 문법 분석: 통과
- 이 환경에서는 npm 레지스트리 DNS 접근이 차단돼 **`npm install` / Vite 브라우저 빌드까지 직접 실행하지 못했습니다.** 따라서 실제 기기에서 시각적 회귀 테스트가 추가로 필요합니다.

중요: 본 ZIP에는 이전 대화에서 노출된 API 인증키를 넣지 않았습니다. 노출된 키는 발급기관에서 재발급하는 것이 안전합니다.
