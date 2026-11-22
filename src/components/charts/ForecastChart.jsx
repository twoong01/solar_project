import React, {useMemo,useState} from 'react';
import {colors,fmt,hourText,modelIndex,dateText} from '../../utils/numbers.js';
export default function ForecastChart({rows,mode,model,date,site,compare,onCompare}){
 const [hover,setHover]=useState(null);
 const [showTable,setShowTable]=useState(false);
 const series=useMemo(()=>{
   const list=[{name:mode==='forecast'?'전날 실제':'조회일 실제',index:mode==='forecast'?3:0,color:colors.reference,dashed:true},{name:model,index:modelIndex(model),color:colors[model]}];
   if(compare){const other=model==='GRU'?'TFT':'GRU';list.push({name:other,index:modelIndex(other),color:colors[other]});}
   return list;
 },[mode,model,compare]);
 const w=900,max=Math.max(1,...rows.flatMap(row=>series.map(item=>Number(row[item.index])||0)))*1.15;
 const x=h=>62+h*(w-92)/23;
 const y=v=>268-(Number(v)||0)/max*230;
 const mainPoints=rows.map((r,h)=>`${x(h)},${y(r[modelIndex(model)])}`).join(' ');
 const summary=mode==='forecast'?`${model} 예측과 전날 실제 발전량을 비교합니다. 예측일 실제값은 없습니다.`:`${model} 예측과 조회일 실제 발전량을 비교합니다.${site==='all'?' 오차는 발전소별 시간 오차를 합산합니다.':''}`;
 return <section className="panel chart-panel" aria-labelledby="chart-title">
    <div className="panel-header"><div><h2 id="chart-title">24시간 발전량 흐름</h2><p>{site==='all'?'전체 발전소':`${site} 발전소`} · {dateText(date)}</p></div><label className="compare-toggle"><input type="checkbox" checked={compare} onChange={e=>onCompare(e.target.checked)}/><span>모델 비교</span></label></div>
    <div className="chart-legend">{series.map(s=><span className="legend-item" key={s.name}><i className={`legend-line ${s.dashed?'dashed':''}`} style={{background:s.color}}/>{s.name}</span>)}</div>
    <div className="chart-container"><svg viewBox={`0 0 ${w} 320`} role="img" aria-label="24시간 발전량 비교 그래프" onPointerLeave={()=>setHover(null)}>
      <defs><linearGradient id="forecast-area" x1="0" x2="0" y1="0" y2="1"><stop offset="0%" stopColor={colors[model]} stopOpacity=".14"/><stop offset="100%" stopColor={colors[model]} stopOpacity=".005"/></linearGradient></defs>
      {Array.from({length:5},(_,i)=>{const v=max*i/4,yy=y(v);return <g key={i}><line x1="62" y1={yy} x2="870" y2={yy} stroke="#e9eef1" strokeDasharray="3 5"/><text x="49" y={yy+4} textAnchor="end" fontSize="13" fill="#788a95">{fmt(v,v>=100?0:1)}</text></g>;})}
      {[0,3,6,9,12,15,18,21,23].map(h=><text key={h} x={x(h)} y="298" textAnchor="middle" fontSize="13" fill="#788a95">{String(h).padStart(2,'0')}시</text>)}
      <polygon points={`${x(0)},268 ${mainPoints} ${x(23)},268`} fill="url(#forecast-area)"/>
      {series.map(s=><polyline key={s.name} points={rows.map((r,h)=>`${x(h)},${y(r[s.index])}`).join(' ')} stroke={s.color} strokeWidth={s.name===model?3:2} fill="none" strokeLinejoin="round" strokeLinecap="round" strokeDasharray={s.dashed?'5 6':undefined}/>) }
      {hover!==null&&<line x1={x(hover)} y1="30" x2={x(hover)} y2="268" stroke="#7e939e" strokeDasharray="3 3"/>}
      {rows.map((row,h)=><rect key={h} className="hour-hit" x={x(h)-17} y="25" width="34" height="250" fill="transparent" tabIndex="0" onPointerEnter={()=>setHover(h)} onFocus={()=>setHover(h)} onBlur={()=>setHover(null)} aria-label={`${hourText(h)}, ${series.map(s=>`${s.name} ${fmt(row[s.index])}`).join(', ')}`}/>)}
    </svg>{hover!==null&&rows[hover]&&<div className="tooltip" style={{left:`${Math.min(77,Math.max(2,x(hover)/9))}%`,top:12}}><strong>{hourText(hover)}</strong>{series.map(s=><div key={s.name}><span>{s.name}</span><span>{fmt(rows[hover][s.index])}</span></div>)}</div>}</div>
    <p className="chart-summary">{summary}</p>
    <div className="chart-footer"><span>발전량 · 원자료 단위</span><button type="button" className="text-button" onClick={()=>setShowTable(v=>!v)} aria-expanded={showTable}>{showTable?'시간별 수치 접기':'시간별 수치 보기'}</button></div>
    {showTable&&<div className="table-scroll"><table><caption className="sr-only">선택 조건의 시간별 발전량</caption><thead><tr><th>시간</th><th>{mode==='forecast'?'전날 실제':'조회일 실제'}</th><th>GRU 예측</th><th>TFT 예측</th></tr></thead><tbody>{rows.map((r,h)=><tr key={h}><th scope="row">{hourText(h)}</th><td>{fmt(r[mode==='forecast'?3:0])}</td><td>{fmt(r[1])}</td><td>{fmt(r[2])}</td></tr>)}</tbody></table></div>}
 </section>;
}
