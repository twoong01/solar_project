"""KPX solar-only odcloud adapter.

Dataset 15065269 is annual/provincial hourly energy trading (MWh), NOT
live observations from the original 13 KHDC inverter sites. Never mix it
into the pretrained 13-site GRU/TFT's inference input.
"""
from __future__ import annotations
import csv
import json
import os
import re
from pathlib import Path
from urllib.parse import unquote, urlencode
from urllib.request import Request, urlopen
from urllib.error import HTTPError, URLError
from datetime import datetime

# Dataset-specific UDDI path verified against public data API registry.
API_URL = 'https://api.odcloud.kr/api/15065269/v1/uddi:2d675f57-b442-497b-a973-e35746c338b7'
BASE = Path(__file__).resolve().parent
SAMPLE = BASE / 'data' / 'kpx_solar_sample.csv'
REGIONS = ('강원도','경기도','경상남도','경상북도','광주시','대구시','대전시','부산시','서울시','세종시','울산시','인천시','전라남도','전라북도','제주도','충청남도','충청북도')
DATE_RX = re.compile(r'^\d{4}-\d{2}-\d{2}$')

class KPXError(Exception): pass


def decode_key(raw: str) -> str:
    """Accept either percent-encoded or decoded public-data keys, once only."""
    if not isinstance(raw, str) or len(raw) > 600 or len(raw) < 12:
        raise KPXError('인증키 형식이 올바르지 않습니다.')
    val = unquote(raw.strip()) if '%' in raw else raw.strip()
    if '\n' in val or '\r' in val or ' ' in val:
        raise KPXError('인증키 형식이 올바르지 않습니다.')
    return val


def validate_query(date: str, region: str):
    if not DATE_RX.fullmatch(date or ''):
        raise KPXError('날짜는 YYYY-MM-DD 형식이어야 합니다.')
    try:
        if datetime.strptime(date,'%Y-%m-%d').strftime('%Y-%m-%d') != date:raise ValueError
    except ValueError:raise KPXError('존재하지 않는 날짜입니다.')
    if region != 'all' and region not in REGIONS:
        raise KPXError('지원하지 않는 시·도입니다.')


def normalize_record(rec: dict):
    if rec.get('연료원') != '태양광':return None
    date=str(rec.get('거래일','')).strip().replace('.','-')
    if len(date)==8 and date.isdigit():date=f'{date[:4]}-{date[4:6]}-{date[6:]}'
    region=str(rec.get('지역','')).strip()
    try:
        hour=int(str(rec.get('거래시간','')).strip())
        energy=float(str(rec.get('전력거래량(MWh)','')).replace(',',''))
    except (ValueError,TypeError):return None
    if not 1 <= hour <= 24 or not (0<=energy<1e12):return None
    return {'date':date,'hour':hour,'region':region,'mwh':energy}


def summarize(rows: list[dict], date: str, region: str, source: str, collected_at: str):
    validate_query(date,region)
    clean=[]
    for record in rows:
        item=normalize_record(record)
        if item and item['date']==date and (region=='all' or item['region']==region):clean.append(item)
    # Dataset 01-24 interval convention is retained: do not relabel as 00-23.
    buckets={h:0. for h in range(1,25)}
    unique=set()
    names=set()
    for item in clean:
        identity=(item['region'],item['hour'])
        if identity in unique:raise KPXError('API 데이터에 중복 지역·시간 행이 있습니다.')
        unique.add(identity)
        buckets[item['hour']]+=item['mwh'];names.add(item['region'])
    return {'date':date,'region':region,'source':source,
            'isRealtime':False,'updatedAt':collected_at,'sourceLastPublished':'2025-12-31',
            'unit':'MWh','fuel':'태양광','intervals':[{'hour':k,'mwh':round(v,6)} for k,v in buckets.items()],
            'totalMWh':round(sum(buckets.values()),6),'records':len(clean),
            'regions':sorted(names),'complete':bool(names) and len(unique)==24*len(names),
            'note':'전력거래소 시·도별 전력시장 거래량: 연간 갱신 공개 데이터. 기존 13개 발전소 관측값 또는 실시간 데이터가 아닙니다.'}


def load_sample(date,region):
    with SAMPLE.open(encoding='utf-8-sig',newline='') as f:
        data=list(csv.DictReader(f))
    return summarize(data,date,region,'사용자 제공 CSV 오프라인 샘플',datetime.now().astimezone().isoformat(timespec='seconds'))


def fetch_kpx(date, region, key, opener=urlopen):
    """Request ONLY solar rows: server-side filters + strict local fuel/date checks.

    Authorization header avoids putting the private key in URLs or access logs.
    Pagination is intentionally bounded; nothing silently claims a fresh feed.
    """
    validate_query(date,region)
    key=decode_key(key)
    conditions={'page':1,'perPage':500,'returnType':'json',
                'cond[연료원::EQ]':'태양광','cond[거래일::EQ]':date}
    if region!='all':conditions['cond[지역::EQ]']=region
    records=[];count=None
    for page in range(1,12):
        conditions['page']=page
        request=Request(API_URL+'?'+urlencode(conditions),
                        headers={'Authorization':'Infuser '+key,'Accept':'application/json','User-Agent':'SolarView-Local/1.0'},
                        method='GET')
        try:
            with opener(request,timeout=18) as response:
                payload=json.loads(response.read(5*1024*1024).decode('utf-8'))
        except HTTPError as exc:
            msg={401:'인증키를 확인하세요.',403:'API 활용신청 승인 및 권한을 확인하세요.',429:'API 호출 한도가 초과되었습니다.'}.get(exc.code,'전력거래소 API에서 오류가 발생했습니다.')
            raise KPXError(f'{msg} (HTTP {exc.code})') from None
        except (URLError,TimeoutError,OSError):
            raise KPXError('API 서버 연결에 실패했습니다. 인터넷 연결 또는 인증 상태를 확인하세요.') from None
        except (ValueError,UnicodeError):
            raise KPXError('API 응답을 JSON으로 읽을 수 없습니다.') from None
        if not isinstance(payload,dict) or not isinstance(payload.get('data'),list):
            raise KPXError('API 응답 구조가 올바르지 않습니다. 활용명세 또는 엔드포인트를 확인하세요.')
        if 'matchCount' in payload:
            try:count=int(payload['matchCount'])
            except (TypeError,ValueError):pass
        batch=payload['data'];records.extend(batch)
        if len(batch)<500 or (count is not None and page*500>=count):break
    if len(records)>=5500:
        raise KPXError('결과가 너무 많아 수집을 중단했습니다. API 조건검색 지원을 확인하세요.')
    result=summarize(records,date,region,'공공데이터포털 odcloud API',datetime.now().astimezone().isoformat(timespec='seconds'))
    # Don't confuse empty response with live forecast.
    if not result['records']:
        raise KPXError(f'{date} 태양광 데이터가 이 API에 없습니다. 공개 범위는 2025년 자료이며 연간 갱신됩니다.')
    if not result['complete']:
        result['note']+=' 선택 날짜에 24시간 일부가 누락되었습니다.'
    return result
