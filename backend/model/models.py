import torch
from torch import nn
import lightning.pytorch as pl

class HourlyGRU(pl.LightningModule):
    """Past 72 hourly observations -> 24 next-day values, with site embedding."""
    def __init__(self,input_size,n_sites,known_indices,hidden_size=32,embedding_size=4,learning_rate=0.001):
        super().__init__();self.save_hyperparameters()
        self.embedding=nn.Embedding(n_sites,embedding_size)
        self.gru=nn.GRU(input_size+embedding_size,hidden_size,batch_first=True)
        self.head=nn.Sequential(nn.Linear(hidden_size+embedding_size+len(known_indices),32),nn.ReLU(),nn.Dropout(.1),nn.Linear(32,1),nn.Softplus())
    def forward(self,x):
        e=self.embedding(x['encoder_cat'][:,0,0]); length=x['encoder_cont'].shape[1]
        _,h=self.gru(torch.cat([x['encoder_cont'],e[:,None,:].expand(-1,length,-1)],-1))
        horizon=x['decoder_cont'].shape[1]
        c=torch.cat([h[-1],e],-1)[:,None,:].expand(-1,horizon,-1)
        return self.head(torch.cat([c,x['decoder_cont'][:,:,self.hparams.known_indices]],-1)).squeeze(-1)
    def step(self,batch,stage):
        x,y=batch;loss=(self(x)-y[0]).abs().mean()
        self.log(stage+'_loss',loss,on_step=False,on_epoch=True,batch_size=len(y[0]));return loss
    def training_step(self,batch,batch_idx):return self.step(batch,'train')
    def validation_step(self,batch,batch_idx):return self.step(batch,'val')
    def configure_optimizers(self):
        opt=torch.optim.AdamW(self.parameters(),lr=self.hparams.learning_rate,weight_decay=1e-4)
        return {'optimizer':opt,'lr_scheduler':{'scheduler':torch.optim.lr_scheduler.ReduceLROnPlateau(opt,patience=3,factor=.5),'monitor':'val_loss'}}
