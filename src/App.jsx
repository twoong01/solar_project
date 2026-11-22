import React,{useEffect,useMemo,useState} from 'react';
import Sidebar from './components/Sidebar.jsx';
import TopBar from './components/TopBar.jsx';
import PageHeading from './components/PageHeading.jsx';
import ForecastPage from './pages/ForecastPage.jsx';
import KpxPage from './pages/KpxPage.jsx';
import KdhcPage from './pages/KdhcPage.jsx';
import InfoDialog from './components/dialogs/InfoDialog.jsx';
import DataDialog from './components/dialogs/DataDialog.jsx';
import {loadForecastData,postJson,backendRequest} from './services/dataService.js';
import {validateForecastData,getAvailableDates,getGroup} from './utils/forecast.js';
import {downloadCsv} from './utils/download.js';

export default function App(){
 const [data,setData]=useState(null),[error,setError]=useState(''),[page,setPage]=useState('forecast');
 const [site,setSite]=useState('all'),[model,setModel]=useState('GRU'),[date,setDate]=useState(''),[compare,setCompare]=useState(true);
 const [modal,setModal]=useState(null),[feedback,setFeedback]=useState(''),[kpxReload,setKpxReload]=useState(0);
 const dates=useMemo(()=>data?getAvailableDates(data):[],[data]);
 useEffect(()=>{let alive=true;loadForecastData().then(d=>{if(!alive)return;const v=validateForecastData(d);setData(v);setDate(v.forecastDate);}).catch(e=>{if(alive)setError(e.message);});return ()=>{alive=false};},[]);
 function notify(msg){setFeedback(msg);window.setTimeout(()=>setFeedback(''),6500);}
 function importData(value){const d=validateForecastData(value);setData(d);setSite(s=>s==='all'||d.sites.includes(s)?s:'all');setDate(d.forecastDate);setPage('forecast');}
 async function reload(){
   let hasBackend=false;
   try { await backendRequest('/api/health'); hasBackend=true; } catch { /* frontend-only */ }
   if(hasBackend) await postJson('/api/refresh',{});
   const d=validateForecastData(await loadForecastData()); importData(d);
 }
 function changeDate(next){if(!data)return;if(!getAvailableDates(data).includes(next)){notify(`${next} 저장 데이터가 없습니다. 조회 범위: ${dates[0]} ~ ${dates.at(-1)}`);return;}setDate(next);setPage(next===data.forecastDate?'forecast':'test');}
 function changePage(target){if(target==='info'||target==='manage'){setModal(target);return;}setPage(target);if(target==='forecast'&&data)setDate(data.forecastDate);if(target==='test'&&data&&(!data.test[date]||date===data.forecastDate))setDate(Object.keys(data.test).sort().at(-1));}
 function downloadForecast(){if(!data||page==='kpx')return;const group=getGroup(data,date),sites=site==='all'?data.sites:[site];downloadCsv(`solar_${page}_${site}_${date}.csv`,[['발전소','날짜','시간','실제_발전량','GRU_예측','TFT_예측','전날_실제_발전량','원자료_보정표시','단위','모델버전'],...sites.flatMap(s=>group[s].map((row,h)=>[s,date,h,row[0],row[1],row[2],row[3],row[4],data.unit,data.modelVersion]))]);}
 async function exit(){if(!window.confirm('Solar View를 종료할까요?'))return;try{await postJson('/api/exit',{});notify('앱 서버 종료 요청을 보냈습니다.');}catch{notify('React 개발 서버는 터미널에서 Ctrl+C로 종료하세요. 브라우저 탭을 닫아도 됩니다.');}}
 const modeName=page==='kdhc'?'발전소 API 진단':page==='kpx'?'태양광 CSV 조회':page==='test'?'모델 검증':'발전량 예측';
 return <>
  <a className="skip" href="#main">본문으로 이동</a>
  <Sidebar active={page} onSelect={changePage}/>
  <div className="shell"><TopBar title={modeName} onExit={exit}/><main id="main"><PageHeading mode={page} onRefresh={()=>page==='kpx'?setKpxReload(n=>n+1):page==='kdhc'?notify('API 조회는 화면의 시험·수집 버튼을 이용하세요.'):setModal('manage')} onDownload={downloadForecast} downloadDisabled={!data}/>
   {feedback&&<div className="feedback" role="status">{feedback}</div>}
   {error&&<div className="error-panel" role="alert">예측 데이터 파일을 불러올 수 없습니다: {error}</div>}
   {page==='kpx'?<KpxPage key={kpxReload}/>:page==='kdhc'?<KdhcPage onPredictComplete={()=>{loadForecastData().then(importData).then(()=>notify('원본 모델 예측 결과를 갱신했습니다.')).catch(e=>notify(e.message));}}/>:data&&date?<ForecastPage {...{data,date,dates,site,model,compare}} mode={page} onDate={changeDate} onSite={setSite} onModel={setModel} onCompare={setCompare}/>:!error&&<div className="panel" style={{padding:24}}>원본 모델 데이터를 불러오는 중입니다…</div>}
   <footer><span>solarview <span className="footer-dot">·</span> 태양광 발전 예측 MVP</span><button type="button" className="text-button" onClick={()=>setModal('info')}>데이터 및 모델 안내</button></footer>
  </main></div>
  <InfoDialog open={modal==='info'} onClose={()=>setModal(null)} data={data}/>
  <DataDialog open={modal==='manage'} onClose={()=>setModal(null)} onImport={importData} onReload={reload} onNotify={notify}/>
 </>;
}
