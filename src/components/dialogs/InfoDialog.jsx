import React from 'react';
import Modal from './Modal.jsx';
import {fmt} from '../../utils/numbers.js';
export default function InfoDialog({open,onClose,data}){
 return <Modal {...{open,onClose}} title="데이터 · 모델 정보"><p>기본 화면은 원본 모델의 저장된 GRU/TFT 예측 결과입니다. 모델 실행 패키지를 설치하고 호환되는 13개 지역 시계열을 제공하면 체크포인트 추론을 실행할 수 있습니다. KPX API 거래량과는 혼합하지 않습니다.</p><dl>
  <dt>예측 대상</dt><dd>지역별 다음 날 00~23시 발전량. 하루 총발전량은 24개 시간값의 합계입니다.</dd>
  <dt>입력 데이터</dt><dd>직전 72시간 발전량과 달력 정보. 기상예보는 사용하지 않습니다.</dd>
  <dt>발전소 구분</dt><dd>13개 발전소 지역에 속한 28개 인버터를 지역별로 합산합니다.</dd>
  <dt>원천 데이터</dt><dd>한국지역난방공사 태양광 운영·기상 CSV · 2025.01.01~2026.03.31</dd>
  <dt>학습 / 검증 / 테스트</dt><dd>학습: 2025.01~09<br/>검증: 2025.10~12<br/>테스트: 2026.01~03<br/>시간 순서대로 분리했으며, 학습 샘플은 72시간 입력 이후부터 시작합니다.</dd>
  <dt>발전량 계산과 단위</dt><dd>인버터별 일 누적 발전량의 시간 차분을 합산합니다. 원본의 물리 단위가 독립적으로 확인되지 않아 ‘원자료 단위’로 표시합니다. 일부 원자료는 정제·보정되었습니다.</dd>
  <dt>WAPE</dt><dd>시간별 |실제 − 예측| 합계 ÷ 실제 발전량 합계 × 100. 오차 지표이며 낮을수록 좋습니다.</dd>
 </dl>{data&&<table className="metrics-dialog-table"><caption>전체 발전소 시간별 WAPE</caption><thead><tr><th>모델</th><th>검증 기간</th><th>테스트 기간</th></tr></thead><tbody>{['GRU','TFT'].map(m=><tr key={m}><th>{m}</th><td>{fmt(data.metrics.validation[m].hourly.WAPE_pct,2)}%</td><td>{fmt(data.metrics.test[m].hourly.WAPE_pct,2)}%</td></tr>)}</tbody></table>}<p className="dialog-note">2026.04.01 화면은 2026.03.31까지 데이터로 생성된 다음 날 예측 예시이며 현재 날짜의 실시간 예측이 아닙니다.</p></Modal>;
}
