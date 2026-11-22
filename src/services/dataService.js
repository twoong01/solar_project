const base = import.meta.env.BASE_URL || '/';
async function fromStatic(file) {
  const response = await fetch(`${base}data/${file}`, {cache:'no-store'});
  if (!response.ok) throw Error(`기본 파일 조회 실패: HTTP ${response.status}`);
  return response.json();
}
export async function loadForecastData() {
  // Production builds served from the optional backend may update /data.json after prediction.
  if (import.meta.env.PROD || import.meta.env.VITE_API_PROXY) {
    try {
      const r = await fetch(`${import.meta.env.BASE_URL || '/'}data.json`, {cache:'no-store'});
      if(r.ok) { const value=await r.json(); if(value?.sites && value?.forecast) return value; }
    } catch { /* static-only hosting falls back to the bundled JSON */ }
  }
  return fromStatic('forecast.json');
}
export const loadSolarTradingData = () => fromStatic('kpx-solar-2025.json');

export async function backendRequest(path, options={}) {
  const response = await fetch(path, {cache:'no-store', ...options});
  let value;
  try { value = await response.json(); } catch { throw Error(`API 응답 형식 오류 (HTTP ${response.status}). 별도 서버가 실행 중인지 확인하세요.`); }
  if(!response.ok) throw Error(value.error || `HTTP ${response.status}`);
  return value;
}
export const postJson = (path, data) => backendRequest(path, {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(data)});
