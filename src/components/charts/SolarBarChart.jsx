import React from 'react';
import {fmt} from '../../utils/numbers.js';
export default function SolarBarChart({intervals=[]}) {
 const values=intervals.map(x=>x.mwh), max=Math.max(1,...values), y0=230;
 return <svg className="solar-react-bars" viewBox="0 0 900 280" role="img" aria-label="시간별 태양광 전력거래량 막대그래프">
  {[0,.25,.5,.75,1].map(r=><g key={r}><line x1="48" x2="872" y1={y0-r*182} y2={y0-r*182} stroke="#e4eaed"/><text x="40" y={y0-r*182+4} textAnchor="end" fill="#92a2ac" fontSize="12">{fmt(max*r,0)}</text></g>)}
  {intervals.map((item,i)=>{const h=182*item.mwh/max,x=56+i*33.5;return <g key={item.hour}><rect x={x} y={y0-h} width="22" height={Math.max(1,h)} rx="4" fill={i>=9&&i<=16?'#e1a45d':'#2a8588'}><title>거래 {item.hour}시: {fmt(item.mwh,2)} MWh</title></rect><text x={x+11} y="249" fontSize="10" textAnchor="middle" fill="#6c808b">{item.hour}</text></g>;})}
  <text x="870" y="272" fontSize="12" textAnchor="end" fill="#7c909c">거래시간 (01~24)</text>
 </svg>;
}
