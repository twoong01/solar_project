import React from 'react';
const items = [
  ['forecast', '◴', '발전량 예측'],
  ['test', '▥', '모델 검증'],
  ['info', 'ⓘ', '데이터 · 모델 정보'],
  ['kpx', '☼', '태양광 발전 실적 · CSV'],
  ['kdhc', '☀', '13개 발전소 API 연결'],
  ['manage', '⟳', '데이터 관리 · 갱신'],
];
export default function Sidebar({active,onSelect}) {
  return <aside className="sidebar">
    <a className="brand" href="./" onClick={e=>{e.preventDefault();onSelect('forecast');}} aria-label="Solar View 홈">
      <span className="brand-icon">☀</span><span>solar<span className="brand-light">view</span><small>ENERGY INTELLIGENCE</small></span>
    </a>
    <div className="workspace"><span className="workspace-icon">S</span><div>태양광 발전소<small>운영 워크스페이스</small></div><span className="mvp">MVP</span></div>
    <p className="nav-label">워크스페이스</p>
    <nav aria-label="주 메뉴">{items.map(([id,icon,label])=><button type="button" key={id} className={`nav-item ${active===id?'active':''}`} onClick={()=>onSelect(id)} aria-current={active===id?'page':undefined}><span aria-hidden="true">{icon}</span>{label}</button>)}</nav>
    <div className="sidebar-bottom"><div className="mini-solar" aria-hidden="true">{Array.from({length:6},(_,i)=><i key={i}/>)}</div><strong>데이터에서, 내일의 에너지로.</strong><p>13개 지역 · 28개 인버터<br/>시간별 발전량을 한눈에 확인하세요.</p><div className="side-version">기존 모델 <span>GRU / TFT</span></div></div>
  </aside>;
}
