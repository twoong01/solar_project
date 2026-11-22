import React from 'react';
export default function ForecastFilters({data,date,dates,site,model,onDate,onSite,onModel}){
  const index=dates.indexOf(date);
  return <section className="filters" aria-label="예측 조회 조건">
    <label className="filter-field">발전소<select aria-label="발전소 선택" value={site} onChange={e=>onSite(e.target.value)}><option value="all">전체 발전소</option>{data.sites.map(s=><option key={s} value={s}>{s} 발전소</option>)}</select></label>
    <div className="date-field"><label className="filter-field">조회 날짜<input type="date" value={date} min={dates[0]} max={dates.at(-1)} onChange={e=>onDate(e.target.value)} aria-label="조회 날짜"/></label><div className="date-nav" aria-label="날짜 이동"><button type="button" disabled={index<=0} onClick={()=>onDate(dates[index-1])} aria-label="이전 데이터 날짜">‹</button><button type="button" disabled={index>=dates.length-1} onClick={()=>onDate(dates[index+1])} aria-label="다음 데이터 날짜">›</button></div></div>
    <fieldset className="model-field"><legend>예측 모델</legend><div className="segmented">{['GRU','TFT'].map(m=><button type="button" key={m} className={model===m?'selected':''} onClick={()=>onModel(m)} aria-pressed={model===m}>{m}</button>)}</div></fieldset>
    <div className="filter-meta"><span>{site==='all'?`${data.sites.length}개 발전소`:`${site} · 인버터 ${data.inverters[site]?.length||0}개`}</span><small>00:00 – 23:00 · 24시간</small></div>
  </section>;
}
