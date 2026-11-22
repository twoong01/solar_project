import React,{useMemo} from 'react';
import ForecastFilters from '../components/ForecastFilters.jsx';
import MetricCards from '../components/MetricCards.jsx';
import ForecastChart from '../components/charts/ForecastChart.jsx';
import Insights from '../components/Insights.jsx';
import PlantTable from '../components/PlantTable.jsx';
import {getGroup,aggregate,rowStats,periodError,wape} from '../utils/forecast.js';
export default function ForecastPage({data,mode,date,dates,site,model,compare,onDate,onSite,onModel,onCompare}){
 const group=useMemo(()=>getGroup(data,date),[data,date]);
 const rows=useMemo(()=>aggregate(group,site),[group,site]);
 const info=useMemo(()=>rowStats(rows,model,mode==='forecast'?3:0),[rows,model,mode]);
 const error=mode==='forecast'?periodError(data,site,model):site==='all'?wape(Object.values(group).flat(),model):wape(rows,model);
 return <>
 <div className="notice"><span className="notice-icon" aria-hidden="true">ⓘ</span><p>{mode==='forecast'?`${data.sourceEnd}까지의 ${data.forecastSource?'실제 모델 추론':'저장된 예측 데이터'}로 만든 ${data.forecastDate} 예측입니다. 자료 갱신 주기와 최신성은 원천 데이터에 따릅니다.`:`${dates[0]}~${data.sourceEnd} 테스트 데이터입니다. 저장된 날짜를 골라 GRU·TFT 예측과 실제를 비교하세요.`}</p></div>
 <ForecastFilters {...{data,date,dates,site,model,onDate,onSite,onModel}}/>
 <div className="dashboard"><MetricCards {...{info,mode,model,date,data,error}}/><div className="chart-grid"><ForecastChart {...{rows,mode,model,date,site,compare,onCompare}}/><Insights {...{rows,info,model}}/></div><PlantTable {...{data,group,site,mode,model,onSite}}/></div>
 </>;
}
