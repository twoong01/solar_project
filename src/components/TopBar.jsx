import React from 'react';
export default function TopBar({title,onExit}){
  return <header className="topbar"><div><span className="breadcrumb">워크스페이스</span><span className="divider">/</span><span>{title}</span></div><div className="topbar-right"><span className="demo-badge"><span aria-hidden="true">◷</span> 데이터 기반 데모</span><button type="button" className="button secondary" onClick={onExit} title="Solar View 실행 종료">앱 종료</button></div></header>;
}
