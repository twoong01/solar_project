"""Invoke original 13-site GRU/TFT model using compatible 72-hour data.
Never pass KPX provincial trading volume into KHDC original model.
"""
from __future__ import annotations
import csv
import io
import json
import os
import re
import subprocess
import sys
import tempfile
from datetime import datetime,timedelta
from pathlib import Path

BASE=Path(__file__).resolve().parent
MODEL=BASE/'model'
META=json.loads((MODEL/'artifacts/preprocessing.json').read_text(encoding='utf-8'))
SITES=list(META['scales'])
DATE_RE=re.compile(r'^\d{4}-\d{2}-\d{2}$')
class ModelInputError(Exception):pass


def validate_input_csv(content: str, as_of: str):
    if not DATE_RE.fullmatch(as_of or ''):raise ModelInputError('asOf 날짜 형식이 올바르지 않습니다.')
    try:day=datetime.strptime(as_of,'%Y-%m-%d')
    except ValueError:raise ModelInputError('날짜 형식이 올바르지 않습니다.')
    if not isinstance(content,str) or len(content)>15_000_000:raise ModelInputError('CSV 파일 크기 제한을 초과했습니다.')
    reader=csv.DictReader(io.StringIO(content.lstrip('\ufeff')))
    if not reader.fieldnames or not {'site','timestamp','energy'}.issubset(reader.fieldnames):
        raise ModelInputError('CSV 열은 site,timestamp,energy 여야 합니다.')
    rows={s:{} for s in SITES}
    end=day+timedelta(hours=23);start=end-timedelta(hours=71)
    for row in reader:
        site=row.get('site','').strip()
        if site not in rows:raise ModelInputError(f'학습 모델에 없는 지역명: {site[:35]}')
        try:
            dt=datetime.fromisoformat(row['timestamp'].strip())
            en=float(row['energy'])
        except (ValueError,TypeError,KeyError):raise ModelInputError('날짜·발전량 값이 올바르지 않습니다.')
        if dt.tzinfo is not None or dt.minute or dt.second or dt.microsecond:
            raise ModelInputError('timestamp는 시간단위 로컬 시간(분·초 0)이어야 합니다.')
        if not 0<=en<1e9:raise ModelInputError('energy는 음수가 아닌 유한한 숫자여야 합니다.')
        if start<=dt<=end:
            if dt in rows[site]:raise ModelInputError(f'{site}: 중복 관측 {dt}')
            rows[site][dt]=en
    expected=[start+timedelta(hours=i) for i in range(72)]
    for site in SITES:
        if len(rows[site])!=72 or any(t not in rows[site] for t in expected):
            raise ModelInputError(f'{site}: {as_of} 23시까지 연속 72시간 발전량이 필요합니다. 현재 {len(rows[site])}시간')
    normalized=[{'site':site,'timestamp':t.strftime('%Y-%m-%d %H:%M:%S'),'energy':rows[site][t]} for site in SITES for t in expected]
    return normalized


def read_historical_csv(as_of: str):
    data=MODEL/'data/hourly_all.csv'
    if not data.exists():raise ModelInputError('기존 모델 시간별 원본 CSV 파일이 없습니다.')
    try:day=datetime.strptime(as_of,'%Y-%m-%d')
    except (TypeError,ValueError):raise ModelInputError('asOf는 YYYY-MM-DD 날짜여야 합니다.')
    start=day-timedelta(days=2)
    kept=[]
    with data.open(encoding='utf-8-sig',newline='') as f:
        reader=csv.DictReader(f)
        for item in reader:
            ts=item.get('timestamp','')
            if start.strftime('%Y-%m-%d')<=ts[:10]<=as_of:
                kept.append({'site':item.get('site',''),'timestamp':ts,'energy':item.get('energy','')})
    output=io.StringIO()
    writer=csv.DictWriter(output,fieldnames=['site','timestamp','energy']);writer.writeheader();writer.writerows(kept)
    return output.getvalue()


def run_model(as_of: str,csv_text: str):
    rows=validate_input_csv(csv_text,as_of)
    # Use original predict.py & original checkpoint files. Requires original runtime libs.
    with tempfile.TemporaryDirectory(prefix='solarview_predict_') as tmp:
        input_path=Path(tmp)/'input.csv';out_path=Path(tmp)/'forecast.csv'
        with input_path.open('w',encoding='utf-8',newline='') as f:
            writer=csv.DictWriter(f,fieldnames=['site','timestamp','energy']);writer.writeheader();writer.writerows(rows)
        cmd=[sys.executable,str(MODEL/'predict.py'),'--hourly',str(input_path),'--as-of',as_of,'--output',str(out_path)]
        try:proc=subprocess.run(cmd,cwd=MODEL,capture_output=True,text=True,timeout=120)
        except subprocess.TimeoutExpired:raise ModelInputError('모델 추론이 제한시간을 초과했습니다.') from None
        if proc.returncode:
            msg=proc.stderr[-1300:]
            if 'No module named' in msg or 'ModuleNotFoundError' in msg:
                raise ModelInputError('AI 실행 패키지가 없습니다. python -m pip install -r model/requirements.txt 설치 후 다시 시도하세요.')
            raise ModelInputError('원본 모델 추론 실패: '+msg[-350:].replace('\n',' '))
        with out_path.open(encoding='utf-8-sig',newline='') as f:forecast=list(csv.DictReader(f))
    if len(forecast)!=13*24:raise ModelInputError(f'추론 결과 개수 오류: {len(forecast)}')
    bysite={s:[None]*24 for s in SITES}
    for row in forecast:
        site=row['site'];hour=int(row['hour'])
        if site not in bysite or hour not in range(24) or bysite[site][hour] is not None:raise ModelInputError('추론 결과 지역/시간 오류')
        bysite[site][hour]=[None,float(row['GRU']),float(row['TFT']),0.,None]
    for site in SITES:
        for h in range(24):bysite[site][h][3]=next(row['energy'] for row in rows if row['site']==site and row['timestamp']==as_of+f' {h:02d}:00:00')
        if any(x is None for x in bysite[site]):raise ModelInputError('24시간 예측 결과 누락')
    target=(datetime.strptime(as_of,'%Y-%m-%d')+timedelta(days=1)).strftime('%Y-%m-%d')
    # Preserve only existing test data, replacing old static forecast; avoid false actuals.
    path=BASE/'dist/data.json';payload=json.loads(path.read_text(encoding='utf-8'))
    payload.update({'forecast':bysite,'forecastDate':target,'sourceEnd':as_of,
        'forecastSource':'원본 GRU/TFT 체크포인트로 재추론',
        'forecastGeneratedAt':datetime.now().astimezone().isoformat(timespec='seconds')})
    from export_data import write_data_json
    write_data_json(payload)
    return {'ok':True,'forecastDate':target,'sourceEnd':as_of,'sites':len(SITES),'rows':len(forecast),'message':'GRU/TFT 원본 체크포인트 추론 완료. 예측 화면이 갱신되었습니다.'}
