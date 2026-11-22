import React,{useState} from 'react';
import Modal from './Modal.jsx';
import {backendRequest,postJson} from '../../services/dataService.js';
import {validateForecastData} from '../../utils/forecast.js';
export default function DataDialog({open,onClose,onImport,onReload,onNotify}){
 const [status,setStatus]=useState('정적 React 화면에서는 저장된 JSON을 다시 불러옵니다.'),[importStatus,setImportStatus]=useState(''),[predictStatus,setPredictStatus]=useState('Python 3.12 / PyTorch 모델 실행 서버 필요'),[asOf,setAsOf]=useState('2026-03-31'),[busy,setBusy]=useState(false);
 async function loadJsonFile(event){const file=event.target.files?.[0];if(!file)return;try{if(file.size>25*1024*1024)throw Error('25MB 이하 JSON만 지원합니다.');const next=validateForecastData(JSON.parse(await file.text()));let persisted=false;
  try { await postJson('/api/data',next);persisted=true; } catch { /* static frontend: browser state only */ }
  onImport(next);setImportStatus(`${file.name} 적용 완료. ${persisted?'백엔드 data.json에도 저장했습니다.':'현재 브라우저에서만 적용됩니다.'}`);onNotify('새 data.json을 화면에 적용했습니다.');}catch(e){setImportStatus(`가져오기 실패: ${e.message}`);}finally{event.target.value='';}}
 async function runModel(csv){setBusy(true);setPredictStatus('별도 Python 모델 서버에 GRU/TFT 추론 요청 중…');try{const payload={asOf};if(csv!==undefined)payload.csv=csv;const res=await postJson('/api/predict',payload);let next;try{next=validateForecastData(await backendRequest('/data.json?now='+Date.now()));}catch{next=null;}
   if(next){onImport(next);onClose();onNotify(`추론 완료: ${res.forecastDate||''}`);}else{setPredictStatus('모델 요청은 처리됐지만 새 data.json을 받지 못했습니다. 백엔드 데이터 경로를 확인하세요.');}
 }catch(e){setPredictStatus(`추론 실행 불가: ${e.message}`);}finally{setBusy(false);}}
 async function uploadPredictCsv(e){const file=e.target.files?.[0];if(!file)return;try{if(file.size>15*1024*1024)throw Error('15MB 이하 CSV만 지원합니다.');await runModel(await file.text());}catch(err){setPredictStatus(err.message);}finally{e.target.value='';}}
 return <Modal {...{open,onClose}} title="데이터 관리 · 갱신"><p>이 화면의 GRU·TFT 예측은 저장된 모델 결과입니다. 날짜를 바꿔도 모델이 다시 학습되거나 실시간 추론이 발생하지 않습니다.</p>
  <div className="data-actions"><h3>① 저장된 데이터 다시 불러오기</h3><p><code>public/data/forecast.json</code>을 다시 요청합니다. React 개발 서버에서는 서버 내부 Python 코드를 자동 실행하지 않습니다.</p><button className="button primary" type="button" onClick={async()=>{try{await onReload();setStatus('기본 JSON 데이터를 새로 불러왔습니다.');onNotify('예측 데이터 새로고침 완료');}catch(e){setStatus(`오류: ${e.message}`)}}}>⟳ 기본 데이터 새로고침</button><p className="action-status">{status}</p></div>
  <div className="data-actions"><h3>② 새 data.json 파일 적용</h3><p>별도로 변환한 <code>data.json</code>을 선택하면 즉시 React 화면에 적용합니다.</p><label className="button upload-button" htmlFor="import-data">⇧ data.json 파일 선택</label><input className="sr-only" id="import-data" type="file" accept="application/json,.json" onChange={loadJsonFile}/><p className="action-status">{importStatus}</p></div>
  <div className="data-actions"><h3>③ 실제 GRU/TFT 체크포인트 추론 (별도 Python 서버)</h3><p>기존 모델과 동일한 13개 지역 <code>site,timestamp,energy</code> CSV가 필요한 기능입니다. 시·도별 KPX 자료는 호환되지 않습니다.</p><label htmlFor="live-as-of">마지막 관측일 (23시까지 필요)</label><input id="live-as-of" type="date" value={asOf} onChange={e=>setAsOf(e.target.value)}/><div className="model-actions"><button type="button" className="button primary" disabled={busy} onClick={()=>runModel()}>⟳ 모델 원본 데이터로 재추론</button><label htmlFor="predict-csv" className="button upload-button">⇧ 13개 지역 CSV 선택·추론</label><input id="predict-csv" className="sr-only" type="file" accept=".csv,text/csv" onChange={uploadPredictCsv}/></div><p className="action-status" role="status">{predictStatus}</p></div>
  <p className="dialog-note">React/Vite 프로젝트는 프론트엔드입니다. API 수집·Python 학습/추론 기능은 별도 백엔드가 실행 중일 때만 사용할 수 있습니다.</p>
 </Modal>;
}
