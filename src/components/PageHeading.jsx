import React from 'react';
export default function PageHeading({mode,onRefresh,onDownload,downloadDisabled}){
 const kpx=mode==='kpx'; const kdhc=mode==='kdhc';
 return <div className="page-heading"><div><p className="eyebrow">SOLAR FORECAST</p><h1>{kdhc?'13개 발전소 실측 API 연결':kpx?'태양광 발전 실적 · 2025 CSV':mode==='test'?'예측과 실제, 한눈에 비교':'시간별 발전량 예측'}</h1><p>{kdhc?'원본 GRU/TFT 모델 입력과의 호환성을 확인합니다.':kpx?'2025년 전체 365일 태양광 데이터를 API 키 없이 조회합니다.':mode==='test'?'테스트 기간의 예측값을 실제 발전량과 비교하세요.':'발전소별 하루 24시간의 발전 흐름을 확인하세요.'}</p></div><div className="heading-actions"><button type="button" className="button secondary" onClick={onRefresh}>⟳ 데이터 갱신</button>{!kpx&&!kdhc&&<button type="button" className="button secondary" onClick={onDownload} disabled={downloadDisabled}>⇩ CSV 다운로드</button>}</div></div>;
}
