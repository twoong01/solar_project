export function downloadCsv(filename, rows) {
  const csv = '\ufeff' + rows.map(row => row.map(value => `"${String(value ?? '').replaceAll('"','""')}"`).join(',')).join('\r\n');
  const url = URL.createObjectURL(new Blob([csv], {type:'text/csv;charset=utf-8'}));
  const a = document.createElement('a'); a.href=url; a.download=filename; document.body.append(a); a.click(); a.remove();
  window.setTimeout(()=>URL.revokeObjectURL(url),1500);
}
