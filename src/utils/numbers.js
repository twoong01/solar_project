export const sum = (values) => values.reduce((a, b) => a + (Number(b) || 0), 0);
export const fmt = (value, digits = 1) => value == null || !Number.isFinite(Number(value))
  ? '—' : Number(value).toLocaleString('ko-KR', {minimumFractionDigits: digits, maximumFractionDigits: digits});
export const dateText = date => `${Number(date.slice(0,4))}년 ${Number(date.slice(5,7))}월 ${Number(date.slice(8,10))}일`;
export const hourText = n => `${String(n).padStart(2, '0')}:00`;
export const modelIndex = model => model === 'TFT' ? 2 : 1;
export const colors = { GRU: '#d57b36', TFT: '#25888c', reference: '#9aa9b1' };
