import React,{useEffect,useRef} from 'react';
export default function Modal({open,onClose,title,children}) {
  const dialog=useRef(null);
  useEffect(()=>{const node=dialog.current;if(!node)return;if(open&&!node.open)node.showModal();if(!open&&node.open)node.close();},[open]);
  return <dialog ref={dialog} onClose={onClose} onCancel={onClose} aria-label={title} onMouseDown={e=>{if(e.target===dialog.current)onClose();}}><div className="dialog-heading"><h2>{title}</h2><button type="button" className="close-button" onClick={onClose} aria-label="닫기">×</button></div><div className="dialog-content">{children}</div></dialog>;
}
