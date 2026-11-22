import React from 'react';
import {fmt,hourText} from '../utils/numbers.js';
export default function MetricCards({info,mode,model,date,data,error}){
 const forecast=mode==='forecast';
 return <section className="stats" aria-label="발전량 요약" aria-live="polite">
  <article className="stat"><div className="stat-label">예측 하루 총발전량<span className="stat-icon orange" aria-hidden="true">ϟ</span></div><div className="stat-value">{fmt(info.total)}</div><div className="stat-foot">{model} · 원자료 단위 · 24시간 합계</div></article>
  <article className="stat"><div className="stat-label">{forecast?'전날 실제 발전량':'조회일 실제 발전량'}<span className="stat-icon" aria-hidden="true">▥</span></div><div className="stat-value">{fmt(info.actual)}</div><div className="stat-foot">{(forecast?data.sourceEnd:date).replaceAll('-','.')} · 원자료 단위</div></article>
  <article className="stat"><div className="stat-label">예측 피크 시간<span className="stat-icon" aria-hidden="true">◷</span></div><div className="stat-value">{info.total>0?hourText(info.peak):'—'}</div><div className="stat-foot">시간당 최대 {fmt(info.peakValue)} · 원자료 단위</div></article>
  <article className="stat"><div className="stat-label">{forecast?'테스트 기간 오차':'조회일 시간별 오차'}<span className="stat-icon" aria-hidden="true">◎</span></div><div className="stat-value">{error==null?'—':<>{fmt(error)}<small>%</small></>}</div><div className="stat-foot">{forecast?'2026.01~03 · 시간별 WAPE':'시간별 WAPE · 낮을수록 좋음'}</div></article>
 </section>;
}
