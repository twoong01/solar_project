# 기존 v7 → React 기본 구조 변환표

| v7 파일/화면 | 신규 React 파일 | 변환 방식 |
|---|---|---|
| `dist/index.html` 전체 | `src/App.jsx`, `src/components/*` | 원본 계층·클래스·문구 유지하고 JSX 컴포넌트화 |
| `dist/app.js` 날짜·모델·지역 변경 | `src/App.jsx`, `src/pages/ForecastPage.jsx` | DOM 조작 대신 React `useState` + props |
| `dist/app.js` 예측 선형 차트 | `src/components/charts/ForecastChart.jsx` | SVG 요소를 JSX로 생성, 모델 비교/툴팁 유지 |
| `dist/app.js` 지역·요약·시간 표 | `MetricCards`, `Insights`, `PlantTable` | 데이터 계산을 `src/utils/forecast.js`로 분리 |
| `dist/kpx.js` 실적 조회 | `src/pages/KpxPage.jsx` | 인증키 없이 2025년 내장 태양광 JSON 직접 조회 |
| `dist/styles.css` | `src/styles/original.css` | 기존 스타일 파일 그대로 복사, `react.css`로 최소 보정 |
| HTML 팝업 | `src/components/dialogs/*` | `<dialog>`를 React props와 ref로 제어 |
| 13개 발전소 API (이전 v9) | `src/pages/KdhcPage.jsx` | 기존 v9 실측 API/72h 수집 진단을 독립 메뉴로 구현 |
| Go / Python API 서버 | `backend/` | v9 API 엔드포인트 유지, React Vite asset 제공 가능하도록 Go 서버 정적 라우팅 확장 |

변경되지 않은 데이터 정의: 원자료 단위, WAPE 산식, 13개 지역, 28개 인버터, GRU/TFT 저장 예측 파일.

**새로운 서비스 기능으로 실제 최신 발전량 예측이 추가된 것은 아닙니다.** 이번 작업 범위는 React 프로젝트로의 구조 변경과 기존 기능의 컴포넌트화입니다.
