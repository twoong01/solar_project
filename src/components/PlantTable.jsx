import React from 'react';
import {fmt,modelIndex,colors} from '../utils/numbers.js';
import {rowStats,wape} from '../utils/forecast.js';
export default function PlantTable({data,group,site,mode,model,onSite}){
 const forecast=mode==='forecast';
 const corrected=forecast?0:Object.values(group).flat().reduce((acc,row)=>acc+(Number(row[4])||0),0);
 return <section className="panel plant-panel" aria-labelledby="plant-title"><div className="panel-header"><div><h2 id="plant-title">발전소별 현황</h2><p>지역별로 합산한 하루 발전량입니다. 발전소를 선택해 자세히 볼 수 있습니다.</p></div><span className="count-tag">{data.sites.length}개 발전소</span></div><div className="table-scroll"><table className="plant-table"><caption className="sr-only">발전소별 예측 총발전량과 기준 발전량</caption><thead><tr><th>발전소</th><th>인버터</th><th>예측 총발전량</th><th>{forecast?'전날 실제 발전량':'조회일 실제 발전량'}</th><th>{forecast?'전날 대비 예측 증감':'시간별 WAPE'}</th><th>예측 분포</th></tr></thead><tbody>{data.sites.map(name=>{
 const r=group[name], info=rowStats(r,model,forecast?3:0),change=info.actual>0?100*(info.total-info.actual)/info.actual:null,metric=forecast?change:wape(r,model);
 const nums=r.map(row=>row[modelIndex(model)]), max=Math.max(1,...nums), points=nums.map((v,i)=>`${2+i*96/23},${24-v/max*22}`).join(' ');
 return <tr key={name} className={site===name?'selected-row':''}><td><button type="button" className="site-button" onClick={()=>{onSite(name);document.getElementById('main')?.scrollIntoView({behavior:'smooth'});}}><span className="site-icon" aria-hidden="true">▦</span>{name}</button></td><td>{data.inverters[name]?.length||0}개</td><td><strong>{fmt(info.total)}</strong></td><td>{fmt(info.actual)}</td><td className="change">{metric===null?'—':`${forecast&&metric>0?'+':''}${fmt(metric)}%`}</td><td><svg className="spark" viewBox="0 0 100 26" aria-hidden="true"><polyline points={points} fill="none" stroke={colors[model]} strokeWidth="1.8"/></svg></td></tr>;
 })}</tbody></table></div><div className="table-note">발전소는 CSV의 ‘발전소 지역’을 기준으로 구분합니다. {forecast?'예측 증감은 전날 실제값 대비 변화이며 오차가 아닙니다.':`조회일 전체 ${data.sites.length*24}개 지역·시간 중 ${corrected}개에 원자료 보정 표시가 있습니다.`}</div></section>;
}
