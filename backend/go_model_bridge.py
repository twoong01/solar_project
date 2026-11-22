"""Bridge from self-contained SolarView.exe to original model inference (optional Python 3.12)."""
import json
import sys
from pathlib import Path
from model_runner import run_model, read_historical_csv

if __name__ == '__main__':
    request = json.loads(Path(sys.argv[1]).read_text(encoding='utf-8'))
    asof = request['asOf']
    text = request.get('csv')
    if text is None:
        text = read_historical_csv(asof)
    print(json.dumps(run_model(asof, text), ensure_ascii=False))
