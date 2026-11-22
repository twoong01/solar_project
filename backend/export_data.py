"""Export only the original hourly GRU/TFT predictions for the static MVP."""
from pathlib import Path
import csv
import hashlib
import json

HERE = Path(__file__).resolve().parent
def source_candidates():
    import os
    env = os.environ.get('SOLAR_ARTIFACTS_DIR')
    candidates = [Path(env).expanduser()] if env else []
    candidates += [
        HERE / 'model' / 'artifacts',
        HERE / 'outputs' / 'solar_hourly_models' / 'artifacts',
        HERE.parent / 'outputs' / 'solar_hourly_models' / 'artifacts',
        HERE.parents[1] / 'outputs' / 'solar_hourly_models' / 'artifacts',
        Path.cwd() / 'outputs' / 'solar_hourly_models' / 'artifacts',
    ]
    return list(dict.fromkeys(path.resolve() for path in candidates))

REQUIRED = ['preprocessing.json','metrics.json','site_metrics.csv', 'test_hourly_predictions.csv', 'next_day_hourly_forecast.csv', 'GRU/best.ckpt', 'TFT/best.ckpt']

def discover_source():
    for path in source_candidates():
        if all((path / name).is_file() for name in REQUIRED):
            return path
    return None

SOURCE = discover_source()

def write_data_json(result):
    data_dir = HERE / 'dist'
    payload = json.dumps(result, ensure_ascii=False, separators=(',', ':'), allow_nan=False)
    for filename, content in [
        ('data.json', payload),
        ('data.js', '/* Offline saved data fallback */\nwindow.__SOLAR_DATA__ = ' + payload + ';\n'),
    ]:
        temporary = data_dir / (filename + '.tmp')
        temporary.write_text(content, encoding='utf-8')
        temporary.replace(data_dir / filename)

def ensure_source():
    global SOURCE
    SOURCE = discover_source()
    if SOURCE is None:
        folders = '\n'.join(str(p) for p in source_candidates())
        raise FileNotFoundError(f'원본 모델 파일을 찾을 수 없습니다. artifacts 폴더를 아래 경로 중 하나에 배치하세요:\n{folders}')
    return SOURCE

def read_csv(name):
    with (SOURCE / name).open(encoding='utf-8-sig', newline='') as f:
        return list(csv.DictReader(f))

def export():
    ensure_source()
    meta = json.loads((SOURCE / 'preprocessing.json').read_text(encoding='utf-8'))
    result = {
        'sites': list(meta['scales']),
        'columns': ['actual', 'GRU', 'TFT', 'previousDay', 'corrected'],
        'test': {}, 'forecast': {},
        'metrics': json.loads((SOURCE / 'metrics.json').read_text(encoding='utf-8')),
        'inverters': meta['site_to_inverters'],
        'sourceEnd': '2026-03-31', 'forecastDate': '2026-04-01',
        'modelVersion': 'original-72h', 'unit': '원자료 단위',
        'lookbackHours': 72, 'horizonHours': 24,
        'siteMetrics': read_csv('site_metrics.csv'),
        'checksums': {
            name: hashlib.sha256((SOURCE / name).read_bytes()).hexdigest()
            for name in ['GRU/best.ckpt', 'TFT/best.ckpt', 'test_hourly_predictions.csv', 'next_day_hourly_forecast.csv']
        },
    }
    for row in read_csv('test_hourly_predictions.csv'):
        hours = result['test'].setdefault(row['date'], {}).setdefault(row['site'], [None] * 24)
        hour = int(row['hour'])
        assert hours[hour] is None
        hours[hour] = [round(float(row[c]), 6) for c in ['energy', 'GRU', 'TFT', 'previous_day_same_hour']] + [int(row['target_corrected'])]
    for row in read_csv('next_day_hourly_forecast.csv'):
        assert row['date'] == result['forecastDate']
        hour = int(row['hour'])
        hours = result['forecast'].setdefault(row['site'], [None] * 24)
        assert hours[hour] is None
        previous = result['test'][result['sourceEnd']][row['site']][hour][0]
        hours[hour] = [None, round(float(row['GRU']), 6), round(float(row['TFT']), 6), previous, None]
    assert len(result['test']) == 90 and len(result['sites']) == 13
    for group in [*result['test'].values(), result['forecast']]:
        assert set(group) == set(result['sites'])
        assert all(len(hours) == 24 and all(h is not None for h in hours) for hours in group.values())
    write_data_json(result)
    print('Exported original predictions: 90 test days + 1 forecast day, 13 sites, 24 hours each.')

if __name__ == '__main__':
    export()
