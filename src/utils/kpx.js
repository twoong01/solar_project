import {sum} from './numbers.js';
export function selectSolarTrading(raw, date, region) {
  const day=raw.days[date];
  if(!day) throw Error(`${date}의 내장 태양광 데이터가 없습니다. 2025년 날짜를 선택해 주세요.`);
  const indexes=region==='all'?raw.regions.map((_,i)=>i):[raw.regions.indexOf(region)];
  if(indexes.includes(-1)) throw Error('조회할 수 없는 지역입니다.');
  const hours=Array.from({length:24},(_,h)=>({hour:h+1, mwh:sum(indexes.map(i=>day[i][h]))}));
  return {date,region,regions:indexes.map(i=>raw.regions[i]),intervals:hours,totalMWh:sum(hours.map(item=>item.mwh)),records:indexes.length*24,source:'내장 CSV · 실제 실적',updatedAt:'2025-12-31',note:'전력거래소 2025년 태양광 전력거래량(MWh). 시도별 집계 자료로 기존 13개 발전소 모델 예측과 별개입니다.',complete:hours.length===24};
}
