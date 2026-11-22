import argparse,json,sys
from pathlib import Path
import pandas as pd
import numpy as np
import torch
from pytorch_forecasting import TimeSeriesDataSet,TemporalFusionTransformer
from models import HourlyGRU
from train import calendar,predictions

ROOT=Path(__file__).resolve().parent

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--hourly',default=str(ROOT/'data/hourly_all.csv'));ap.add_argument('--as-of',default='2026-03-31');ap.add_argument('--output',default=str(ROOT/'artifacts/next_day_hourly_forecast.csv'));args=ap.parse_args()
    torch.set_num_threads(4)
    meta=json.loads((ROOT/'artifacts/preprocessing.json').read_text(encoding='utf-8'))
    d=pd.read_csv(args.hourly,parse_dates=['timestamp'])
    cutoff=pd.Timestamp(args.as_of).normalize()+pd.Timedelta(hours=23);start=cutoff+pd.Timedelta(hours=1)
    frames=[]
    for site in meta['scales']:
        h=d[(d.site==site)&(d.timestamp<=cutoff)].sort_values('timestamp').tail(meta['lookback_hours']).copy()
        if len(h)!=meta['lookback_hours'] or h.timestamp.max()!=cutoff or not h.timestamp.diff().dropna().eq(pd.Timedelta(hours=1)).all():raise ValueError(f'{site}: need 72 consecutive hours ending {cutoff}')
        future=pd.concat([h.iloc[[-1]]]*24,ignore_index=True)
        future['timestamp']=pd.date_range(start,periods=24,freq='h');future['energy']=0.;future['energy_scaled']=0.
        frames.append(pd.concat([h,future],ignore_index=True))
    frame=pd.concat(frames,ignore_index=True)
    frame['time_idx']=((frame.timestamp-pd.Timestamp(meta['origin'])).dt.total_seconds()/3600).astype(int)
    frame['energy_scaled']=frame.energy/frame.site.map(meta['scales']);calendar(frame)
    params=torch.load(ROOT/'artifacts/dataset_parameters.pt',map_location='cpu',weights_only=False)
    ds=TimeSeriesDataSet.from_parameters(params,frame,predict=True,stop_randomization=True)
    idx=ds.decoded_index
    result=pd.DataFrame({'site':np.repeat(idx.site.to_numpy(),24),'date':str(start.date()),'hour':np.tile(np.arange(24),len(idx))})
    for name,cls in [('GRU',HourlyGRU),('TFT',TemporalFusionTransformer)]:
        ck=torch.load(ROOT/f'artifacts/{name}/best.ckpt',map_location='cpu',weights_only=False)
        model=cls(**ck['hyper_parameters']);model.load_state_dict(ck['state_dict'])
        result[name]=predictions(model,ds,name).reshape(-1)*result.site.map(meta['scales']).to_numpy()
    output=Path(args.output);output.parent.mkdir(parents=True,exist_ok=True)
    result.to_csv(output,index=False,encoding='utf-8-sig')
    result.groupby(['site','date'])[['GRU','TFT']].sum().reset_index().to_csv(output.with_name(output.stem+'_daily_sum.csv'),index=False,encoding='utf-8-sig')
    print(f'{len(result)} hourly forecasts saved to {output}');print(result.head(24).to_string(index=False))

if __name__=='__main__':sys.stdout.reconfigure(encoding='utf-8');main()
