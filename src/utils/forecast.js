import {sum, modelIndex} from './numbers.js';

export function validateForecastData(value) {
  if (!value || !Array.isArray(value.sites) || !value.sites.length || !value.test || !value.forecast || !value.inverters || !value.metrics || !value.siteMetrics || !value.forecastDate) throw new Error('예측 데이터 필수 항목이 누락되었습니다.');
  for (const [day, group] of [...Object.entries(value.test), [value.forecastDate, value.forecast]]) {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(day)) throw new Error('날짜 형식이 잘못되었습니다.');
    for (const site of value.sites) {
      if (!Array.isArray(group[site]) || group[site].length !== 24 || !group[site].every(row => Array.isArray(row) && row.length >= 4)) throw new Error(`${day} ${site}: 24시간 데이터가 없습니다.`);
    }
  }
  for (const model of ['GRU','TFT']) {
    if (!Number.isFinite(value.metrics.test?.[model]?.hourly?.WAPE_pct)) throw new Error(`${model} 평가 지표가 없습니다.`);
  }
  return value;
}
export function getAvailableDates(data) {
  return [...new Set([...Object.keys(data.test), data.forecastDate])].sort();
}
export function getGroup(data, date) {
  return date === data.forecastDate ? data.forecast : data.test[date];
}
export function aggregate(group, site) {
  if (!group) return [];
  if (site !== 'all') return group[site] || [];
  const regions = Object.values(group);
  return Array.from({length:24},(_,hour) => Array.from({length:5},(_,col) => {
    const values = regions.map(region => region[hour][col]);
    return values.some(value => value == null) ? null : sum(values);
  }));
}
export function wape(rows, model) {
  if (!rows.length || rows.some(row => row[0] == null)) return null;
  const denom = sum(rows.map(row => row[0]));
  return denom > 0 ? 100 * sum(rows.map(row => Math.abs(row[0] - row[modelIndex(model)]))) / denom : null;
}
export function rowStats(rows, model, actualCol) {
  const pred = rows.map(row => row[modelIndex(model)] || 0);
  const peakValue = Math.max(0, ...pred);
  return {total:sum(pred),actual:sum(rows.map(row => row[actualCol])),peak:pred.indexOf(peakValue),peakValue};
}
export function periodError(data, site, model) {
  if (site === 'all') return data.metrics.test?.[model]?.hourly?.WAPE_pct ?? null;
  const record = data.siteMetrics.find(row => row.split === 'test' && row.model === model && row.site === site);
  return record ? Number(record.hourly_WAPE_pct) : null;
}
