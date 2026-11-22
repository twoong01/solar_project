import argparse,json,hashlib,sys,time,os
from pathlib import Path
import numpy as np
import pandas as pd
import torch
import lightning.pytorch as pl
from lightning.pytorch.callbacks import ModelCheckpoint,EarlyStopping,Callback
from lightning.pytorch.loggers import CSVLogger
from pytorch_forecasting import TimeSeriesDataSet,TemporalFusionTransformer
from pytorch_forecasting.metrics import MAE
from models import HourlyGRU

ROOT=Path(__file__).resolve().parent
LOOKBACK=72; HORIZON=24
KNOWN=['hour_sin','hour_cos','year_sin','year_cos']
OBSERVED=['energy_scaled']

def calendar(d):
    d['hour']=d.timestamp.dt.hour
    phase=2*np.pi*d.hour/24
    d['hour_sin']=np.sin(phase);d['hour_cos']=np.cos(phase)
    phase=2*np.pi*(d.timestamp.dt.dayofyear-1)/365.25
    d['year_sin']=np.sin(phase);d['year_cos']=np.cos(phase)

def prepare(path):
    raw=pd.read_csv(path);key=['발전소 지역','인버터 ID','일자']
    assert not raw.duplicated(key+['시간']).any()
    assert raw.groupby(key)['시간'].nunique().eq(24).all()
    raw=raw.sort_values(key+['시간'])
    raw['energy']=raw.groupby(key)['금일발전량'].diff()
    raw.loc[raw['시간']==0,'energy']=raw.loc[raw['시간']==0,'금일발전량']
    assert raw.energy.notna().all() and raw.energy.ge(0).all()
    # A differenced target depends on both the current and previous counter.
    prev=raw.groupby(key)['결측보정여부'].shift().eq('Y')
    raw['target_corrected']=(raw['결측보정여부'].eq('Y')|prev).astype(int)
    d=raw.groupby(['발전소 지역','일자','시간']).agg(energy=('energy','sum'),target_corrected=('target_corrected','max'),inverter_count=('인버터 ID','nunique')).reset_index()
    d=d.rename(columns={'발전소 지역':'site','일자':'date','시간':'hour'})
    d['timestamp']=pd.to_datetime(d.date)+pd.to_timedelta(d.hour,unit='h')
    d=d.sort_values(['site','timestamp']).reset_index(drop=True)
    d['time_idx']=((d.timestamp-d.timestamp.min()).dt.total_seconds()/3600).astype(int)
    d['split']=np.where(d.date<='2025-09-30','train',np.where(d.date<='2025-12-31','validation','test'))
    scale=d[d.split=='train'].groupby('site').energy.mean().clip(lower=1).to_dict()
    d['energy_scaled']=d.energy/d.site.map(scale);calendar(d)
    assert d.groupby('site').time_idx.diff().dropna().eq(1).all()
    counters=raw[raw['시간']==23].groupby(['발전소 지역','일자'])['금일발전량'].sum().sort_index()
    hourly=d.groupby(['site','date']).energy.sum().sort_index()
    np.testing.assert_allclose(counters.to_numpy(),hourly.to_numpy(),rtol=0,atol=0)
    mapping=raw.groupby('발전소 지역')['인버터 ID'].apply(lambda z:sorted(z.unique().tolist())).to_dict()
    meta={'lookback_hours':LOOKBACK,'horizon_hours':HORIZON,'known':KNOWN,'observed':OBSERVED,'scales':scale,'origin':str(d.timestamp.min()),'site_to_inverters':mapping,'source_sha256':hashlib.sha256(Path(path).read_bytes()).hexdigest(),'unit':'source energy unit; not independently verified','scope':'13 regions as site proxies, sum of inverter hourly increments','forecast_origin':'after previous-day hour-23 observations arrive','target':'next calendar day hour labels 0 through 23','seed':42}
    return d,meta

def build_sets(d):
    train=TimeSeriesDataSet(d[d.split=='train'],time_idx='time_idx',target='energy_scaled',group_ids=['site'],min_encoder_length=LOOKBACK,max_encoder_length=LOOKBACK,min_prediction_length=HORIZON,max_prediction_length=HORIZON,static_categoricals=['site'],time_varying_known_reals=KNOWN,time_varying_unknown_reals=OBSERVED,target_normalizer=None,allow_missing_timesteps=False)
    val=TimeSeriesDataSet.from_dataset(train,d[d.date<='2025-12-31'],min_prediction_idx=int(d[d.split=='validation'].time_idx.min()),stop_randomization=True)
    test=TimeSeriesDataSet.from_dataset(train,d,min_prediction_idx=int(d[d.split=='test'].time_idx.min()),stop_randomization=True)
    return {k:v.filter(lambda idx:idx.time_idx_first_prediction.mod(24).eq(0)) for k,v in {'train':train,'validation':val,'test':test}.items()}

def move(x):return {k:v.cpu() if torch.is_tensor(v) else v for k,v in x.items()}

def predictions(model,ds,name):
    model.cpu().eval();all_p=[]
    with torch.inference_mode():
        for x,_ in ds.to_dataloader(train=False,batch_size=64,num_workers=0):
            p=model(x) if name=='GRU' else model(x)['prediction'].squeeze(-1)
            all_p.append(p.numpy())
    return np.maximum(np.concatenate(all_p,axis=0),0)

def metric(y,p):
    y=np.asarray(y);p=np.asarray(p);e=p-y
    return {'MAE':float(abs(e).mean()),'RMSE':float(np.sqrt((e**2).mean())),'WAPE_pct':float(abs(e).sum()/y.sum()*100) if y.sum() else None,'bias':float(e.mean()),'n':len(y)}

def leakage(model,ds,name):
    x,_=next(iter(ds.to_dataloader(train=False,batch_size=8,num_workers=0)))
    model.eval()
    with torch.inference_mode():
        a=model(x) if name=='GRU' else model(x)['prediction']
        x['decoder_target'].fill_(9999)
        x['decoder_cont'][:,:,ds.reals.index('energy_scaled')]=9999
        b=model(x) if name=='GRU' else model(x)['prediction']
    delta=float((a-b).abs().max());assert delta<1e-6;return delta

class Progress(Callback):
    def __init__(self,name):self.name=name
    def on_validation_epoch_end(self,trainer,pl_module):
        if not trainer.sanity_checking:print(f'{self.name} epoch={trainer.current_epoch+1} val_loss={float(trainer.callback_metrics["val_loss"]):.6f}',flush=True)

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--csv',required=True);ap.add_argument('--epochs',type=int,default=40);args=ap.parse_args()
    torch.set_num_threads(4);pl.seed_everything(42,workers=True)
    out=ROOT/'artifacts';out.mkdir(exist_ok=True);(ROOT/'data').mkdir(exist_ok=True)
    d,meta=prepare(args.csv);sets=build_sets(d)
    d.to_csv(ROOT/'data/hourly_all.csv',index=False,encoding='utf-8-sig')
    splits={}
    for name,ds in sets.items():
        d[d.split==name].to_csv(ROOT/f'data/{name}_hourly.csv',index=False,encoding='utf-8-sig')
        idx=ds.decoded_index.copy();assert idx.time_idx_first_prediction.mod(24).eq(0).all()
        idx['target_date']=(pd.Timestamp(meta['origin'])+pd.to_timedelta(idx.time_idx_first_prediction,unit='h')).dt.strftime('%Y-%m-%d')
        idx.to_csv(ROOT/f'data/{name}_windows.csv',index=False,encoding='utf-8-sig')
        splits[name]={'windows':len(ds),'hourly_targets':len(ds)*24,'target_start':idx.target_date.min(),'target_end':idx.target_date.max(),'source_rows':int(d.split.eq(name).sum())}
    assert [len(sets[x]) for x in ['train','validation','test']]==[3510,1196,1170]
    meta['splits']=splits;meta['real_features']=sets['train'].reals
    (out/'preprocessing.json').write_text(json.dumps(meta,ensure_ascii=False,indent=2),encoding='utf-8')
    torch.save(sets['train'].get_parameters(),out/'dataset_parameters.pt')
    nsites=len(sets['train'].get_parameters()['categorical_encoders']['site'].classes_)
    models={'GRU':HourlyGRU(len(sets['train'].reals),nsites,[sets['train'].reals.index(k) for k in KNOWN]),'TFT':TemporalFusionTransformer.from_dataset(sets['train'],hidden_size=16,attention_head_size=2,lstm_layers=1,hidden_continuous_size=8,dropout=.1,learning_rate=.001,output_size=1,loss=MAE(),reduce_on_plateau_patience=3,log_interval=-1)}
    runs={}
    for name,model in models.items():
        pl.seed_everything(42,workers=True)
        ck=ModelCheckpoint(dirpath=out/name,filename='best',monitor='val_loss',mode='min',save_top_k=1)
        trainer=pl.Trainer(max_epochs=args.epochs,accelerator='cpu',devices=1,gradient_clip_val=.5,callbacks=[ck,EarlyStopping(monitor='val_loss',mode='min',patience=6,min_delta=.0005),Progress(name)],logger=CSVLogger(str(out/'logs'),name=name),enable_progress_bar=False,enable_model_summary=False,deterministic=True,num_sanity_val_steps=0)
        start=time.time();trainer.fit(model,train_dataloaders=sets['train'].to_dataloader(train=True,batch_size=64,num_workers=0),val_dataloaders=sets['validation'].to_dataloader(train=False,batch_size=64,num_workers=0))
        saved=torch.load(ck.best_model_path,map_location='cpu',weights_only=False);model.load_state_dict(saved['state_dict'])
        runs[name]={'parameters':sum(p.numel() for p in model.parameters()),'epochs':trainer.current_epoch,'best_epoch':int(saved['epoch'])+1,'seconds':round(time.time()-start,1),'best_val_loss':float(ck.best_model_score),'future_target_perturbation_delta':leakage(model,sets['validation'],name)}
    # All fitting and checkpoint selection is finished before test evaluation.
    summary={'runs':runs,'splits':splits};regional=[]
    d['previous_day_same_hour']=d.groupby('site').energy.shift(24)
    for split in ['validation','test']:
        ds=sets[split];idx=ds.decoded_index
        q=pd.DataFrame({'site':np.repeat(idx.site.to_numpy(),24),'time_idx':(idx.time_idx_first_prediction.to_numpy()[:,None]+np.arange(24)).reshape(-1)})
        q=q.merge(d,on=['site','time_idx'],validate='one_to_one',how='left')
        assert q.split.eq(split).all() and len(q)==len(ds)*24
        for name,model in models.items():q[name]=predictions(model,ds,name).reshape(-1)*q.site.map(meta['scales']).to_numpy()
        daily=q.groupby(['site','date'])[['energy','previous_day_same_hour','GRU','TFT']].sum().reset_index()
        summary[split]={}
        for name in ['previous_day_same_hour','GRU','TFT']:
            r={'hourly':metric(q.energy,q[name]),'daily_sum':metric(daily.energy,daily[name])}
            daylight=q[q.hour.between(6,19)];r['hours_06_19']=metric(daylight.energy,daylight[name])
            clean=q[q.target_corrected.eq(0)];r['uncorrected_differenced_targets']=metric(clean.energy,clean[name])
            for site,z in q.groupby('site'):
                sm=metric(z.energy,z[name]);zz=daily[daily.site==site];dm=metric(zz.energy,zz[name]);regional.append({'split':split,'model':name,'site':site,'hourly_WAPE_pct':sm['WAPE_pct'],'hourly_MAE':sm['MAE'],'daily_WAPE_pct':dm['WAPE_pct']})
            summary[split][name]=r
        q[['site','date','hour','energy','target_corrected','previous_day_same_hour','GRU','TFT']].to_csv(out/f'{split}_hourly_predictions.csv',index=False,encoding='utf-8-sig')
        daily.to_csv(out/f'{split}_daily_sums.csv',index=False,encoding='utf-8-sig')
    summary['selected_neural_by_validation_hourly_WAPE']=min(['GRU','TFT'],key=lambda n:summary['validation'][n]['hourly']['WAPE_pct'])
    (out/'metrics.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2),encoding='utf-8');pd.DataFrame(regional).to_csv(out/'site_metrics.csv',index=False,encoding='utf-8-sig')
    print('FINAL',json.dumps(summary,ensure_ascii=False),flush=True)
    plots(out)

def plots(out):
    import matplotlib;matplotlib.use('Agg')
    import matplotlib.pyplot as plt
    plt.rcParams['font.family']='Malgun Gothic';plt.rcParams['axes.unicode_minus']=False
    p=pd.read_csv(out/'test_hourly_predictions.csv');dates=['2026-01-15','2026-02-15','2026-03-15']
    fig,axs=plt.subplots(3,1,figsize=(12,10),layout='constrained')
    for ax,date in zip(axs,dates):
        z=p[(p.site=='강남')&(p.date==date)]
        for col,label in [('energy','실제'),('GRU','GRU'),('TFT','TFT')]:ax.plot(z.hour,z[col],label=label)
        ax.set_title(f'강남 · {date} · 다음날 0~23시 예측');ax.set_xticks(range(0,24,2));ax.set_ylabel('시간별 발전량 (원단위)');ax.legend()
    fig.savefig(out/'hourly_examples.png',dpi=140);plt.close(fig)

if __name__=='__main__':sys.stdout.reconfigure(encoding='utf-8');main()
