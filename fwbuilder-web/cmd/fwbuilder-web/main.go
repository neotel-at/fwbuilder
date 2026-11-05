package main

import (
    "crypto/subtle"
    "encoding/json"
    "flag"
    "html/template"
    "io"
    "log"
    "net/http"
    "os"
    "strings"
    "sync"
    "time"

    "github.com/neotel-at/fwbuilder/fwbuilder-web/internal/fwbxml"
)

type server struct {
    mu sync.RWMutex
    doc *fwbxml.Document
    filename string
    revision uint64
    token string
    page *template.Template
}

func main() {
    listen:=flag.String("listen","127.0.0.1:8080","HTTP listen address")
    file:=flag.String("file","","optional .fwb file to preload")
    flag.Parse()
    s:=&server{token:os.Getenv("FWBUILDER_WEB_TOKEN"),page:template.Must(template.New("app").Parse(indexHTML))}
    if *file!="" {
        f,err:=os.Open(*file);if err!=nil { log.Fatal(err) };defer f.Close()
        doc,err:=fwbxml.Parse(f);if err!=nil { log.Fatal(err) }
        s.doc=doc;s.filename=*file
    }
    mux:=http.NewServeMux()
    mux.HandleFunc("/",s.index)
    mux.HandleFunc("/api/document",s.document)
    mux.HandleFunc("/api/import",s.importFile)
    mux.HandleFunc("/api/rules",s.rules)
    mux.HandleFunc("/api/policy/edit",s.editPolicy)
    srv:=&http.Server{Addr:*listen,Handler:s.headers(mux),ReadHeaderTimeout:5*time.Second,ReadTimeout:30*time.Second,WriteTimeout:30*time.Second,MaxHeaderBytes:1<<20}
    log.Printf("fwbuilder-web listening on %s",*listen)
    log.Fatal(srv.ListenAndServe())
}

func (s *server) authorized(r *http.Request) bool {
    if s.token=="" { return true }
    auth:=strings.TrimPrefix(r.Header.Get("Authorization"),"Bearer ")
    return subtle.ConstantTimeCompare([]byte(auth),[]byte(s.token))==1
}
func (s *server) headers(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
        w.Header().Set("X-Content-Type-Options","nosniff")
        w.Header().Set("Referrer-Policy","no-referrer")
        w.Header().Set("Content-Security-Policy","default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
        if strings.HasPrefix(r.URL.Path,"/api/") && !s.authorized(r) { http.Error(w,"unauthorized",http.StatusUnauthorized);return }
        next.ServeHTTP(w,r)
    })
}
func jsonResponse(w http.ResponseWriter,v any) {
    w.Header().Set("Content-Type","application/json; charset=utf-8")
    _=json.NewEncoder(w).Encode(v)
}
func (s *server) index(w http.ResponseWriter,r *http.Request){
    if r.URL.Path!="/" {http.NotFound(w,r);return}
    if r.Method!=http.MethodGet { http.Error(w,"method not allowed",405);return }
    w.Header().Set("Content-Type","text/html; charset=utf-8")
    _=s.page.Execute(w,nil)
}
func (s *server) document(w http.ResponseWriter,r *http.Request){
    if r.Method!=http.MethodGet {http.Error(w,"method not allowed",405);return}
    s.mu.RLock();defer s.mu.RUnlock()
    if s.doc==nil {jsonResponse(w,map[string]any{"loaded":false});return}
    jsonResponse(w,map[string]any{"loaded":true,"filename":s.filename,"version":s.doc.Version,"count":s.doc.Count,"revision":s.revision,"root":s.doc.Root})
}
func (s *server) importFile(w http.ResponseWriter,r *http.Request){
    if r.Method!=http.MethodPost {http.Error(w,"method not allowed",405);return}
    // Prevent cross-site uploads even in local unauthenticated mode.
    if origin:=r.Header.Get("Origin");origin!="" {
        if !strings.EqualFold(origin,"http://"+r.Host) && !strings.EqualFold(origin,"https://"+r.Host) {http.Error(w,"cross-origin request rejected",403);return}
    }
    r.Body=http.MaxBytesReader(w,r.Body,fwbxml.MaxDocumentSize)
    err:=r.ParseMultipartForm(4<<20)
    if err!=nil {http.Error(w,"invalid upload: "+err.Error(),400);return}
    f,h,err:=r.FormFile("file")
    if err!=nil {http.Error(w,"missing file",400);return}
    defer f.Close()
    d,err:=fwbxml.Parse(io.LimitReader(f,fwbxml.MaxDocumentSize+1))
    if err!=nil {http.Error(w,"invalid .fwb: "+err.Error(),400);return}
    s.mu.Lock();s.doc=d;s.filename=h.Filename;s.revision++;s.mu.Unlock()
    jsonResponse(w,map[string]any{"loaded":true,"filename":h.Filename,"version":d.Version,"count":d.Count})
}
func (s *server) rules(w http.ResponseWriter,r *http.Request){
    if r.Method!=http.MethodGet {http.Error(w,"method not allowed",405);return}
    id:=r.URL.Query().Get("id")
    s.mu.RLock();defer s.mu.RUnlock()
    if s.doc==nil {http.Error(w,"no file loaded",404);return}
    rules,err:=s.doc.Rules(id)
    if err!=nil {http.Error(w,err.Error(),400);return}
    jsonResponse(w,rules)
}

func (s *server) editPolicy(w http.ResponseWriter,r *http.Request){
    if r.Method!=http.MethodPost {http.Error(w,"method not allowed",405);return}
    if origin:=r.Header.Get("Origin");origin!="" && !strings.EqualFold(origin,"http://"+r.Host) && !strings.EqualFold(origin,"https://"+r.Host) {http.Error(w,"cross-origin request rejected",403);return}
    var req fwbxml.PolicyEdit
    dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,64<<10));dec.DisallowUnknownFields()
    if err:=dec.Decode(&req);err!=nil {http.Error(w,err.Error(),400);return}
    s.mu.Lock();defer s.mu.Unlock()
    if s.doc==nil {http.Error(w,"no file loaded",404);return}
    if req.Revision!=s.revision {http.Error(w,"stale document revision",409);return}
    if err:=s.doc.EditPolicy(req);err!=nil {http.Error(w,err.Error(),400);return}
    s.revision++
    jsonResponse(w,map[string]any{"revision":s.revision})
}

const indexHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>fwbuilder-web</title>
<style>
:root{font:14px system-ui,sans-serif;color:#e5e7eb;background:#101827}
*{box-sizing:border-box}body{margin:0}header{display:flex;gap:16px;align-items:center;padding:14px 20px;background:#172235;border-bottom:1px solid #334155}
h1{font-size:18px;margin:0}button,input{font:inherit}button{background:#2563eb;color:white;border:0;padding:8px 12px;border-radius:6px;cursor:pointer}
input{background:#0f172a;color:#e5e7eb;border:1px solid #475569;border-radius:6px;padding:8px}main{display:flex;min-height:0;height:calc(100vh - 68px);overflow:hidden}#workspace{display:flex;flex-direction:column;min-width:0;min-height:0;flex:1}#upper{display:flex;min-width:0;min-height:0;flex:1}#left-pane{flex:none;width:340px;min-width:170px}#right-pane{flex:1;min-width:180px;overflow:auto}#editor-pane{flex:none;height:260px;min-height:90px;overflow:auto;border-top:1px solid #334155;padding:12px 20px;background:#172235}#editor-pane[hidden],#horizontal-splitter[hidden]{display:none}.splitter{flex:none;background:#334155;touch-action:none;user-select:none;position:relative;z-index:2}.splitter:hover,.splitter.dragging{background:#60a5fa}#vertical-splitter{width:6px;cursor:col-resize}#horizontal-splitter{height:6px;cursor:row-resize}.pane-title{display:flex;align-items:center;gap:10px;justify-content:space-between}.pane-title button{background:#334155;padding:3px 9px}.toolbar-button{background:#334155}body.resizing{cursor:col-resize;user-select:none}body.resizing-y{cursor:row-resize;user-select:none}
aside{overflow:auto;border-right:1px solid #334155;padding:12px}article{padding:10px 12px;overflow:auto}#tree button{background:none;text-align:left;width:100%;color:#dbeafe;padding:4px;border-radius:4px}
#tree button:hover,#tree button.sel{background:#334155}.entry{margin-left:14px;border-left:1px solid #334155;padding-left:7px}
small,.muted{color:#94a3b8}table{border-collapse:collapse;width:100%;margin-top:16px}th,td{border-bottom:1px solid #334155;padding:10px;text-align:left;vertical-align:top}th{color:#93c5fd}pre{white-space:pre-wrap;overflow-wrap:anywhere;background:#172235;padding:12px;border-radius:6px}.toolbar{display:flex;gap:8px;align-items:center;flex-wrap:wrap}
@media(max-width:700px){#left-pane{min-width:110px}#right-pane{min-width:120px}#editor-pane{min-height:75px}}
.policy-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:4px 8px}.policy-heading h3{margin:5px 0;font-size:16px}.policy-heading span{font-size:12px;color:#94a3b8}.policy-scroll{overflow:auto;max-width:100%;border:1px solid #334155}.policy-table{table-layout:auto;border-collapse:separate;border-spacing:0;width:100%;margin:0;font-size:12px}.policy-table th{position:sticky;top:0;z-index:1;background:#243248;color:#cbd5e1;white-space:nowrap;text-align:left;font-size:12px;padding:7px 9px;border-right:1px solid #475569;border-bottom:2px solid #64748b}.policy-table td{padding:5px 8px;border-right:1px solid #334155;border-bottom:1px solid #334155;vertical-align:top}.policy-table .rule-number{min-width:35px;text-align:right;color:#94a3b8}.policy-table .object-cell{min-width:125px}.object-line{display:flex;align-items:center;gap:5px;min-height:22px;white-space:nowrap;cursor:pointer}.object-line:hover{color:#93c5fd}.object-icon{display:inline-flex;align-items:center;justify-content:center;width:19px;height:19px;border-radius:4px;color:#38bdf8;background:#164e63;font-size:14px;flex:none}.policy-row:hover{background:#233149}.policy-row.rule-selected{background:#475569}.policy-row.rule-disabled{opacity:.55}.direction-label{color:#93c5fd;white-space:nowrap}.action-pill{white-space:nowrap;font-weight:600}.action-pill.accept{color:#4ade80}.action-pill.deny{color:#f87171}.action-pill.other{color:#fbbf24}.option-cell{text-align:center;color:#fbbf24}.comment-cell{min-width:160px;max-width:360px;overflow-wrap:anywhere}.rule-controls{white-space:nowrap}.rule-controls button{padding:3px 6px;margin-right:3px;font-size:11px;background:#334155}
</style></head><body><header><h1>🛡 fwbuilder-web</h1><span class="muted">Go • Policy editor (in-memory)</span><input type="file" id="upload" accept=".fwb,.xml"><button id="load">Import .fwb</button><button class="toolbar-button" id="toggle-editor" type="button">Hide Editor</button><button class="toolbar-button" id="reset-layout" type="button">Reset Layout</button><span id="status"></span></header>
<main id="workspace"><div id="upper"><aside id="left-pane"><div class="toolbar"><input id="search" placeholder="Search objects…" aria-label="Search objects" style="width:100%"></div><div id="tree"></div></aside><div id="vertical-splitter" class="splitter" role="separator" aria-label="Resize object tree" aria-orientation="vertical" tabindex="0" aria-valuemin="170" aria-valuenow="340"></div><article id="right-pane"><div id="detail"><h2>Firewall Builder</h2><p class="muted">Import an existing .fwb file to browse its objects and rule sets.</p></div></article></div><div id="horizontal-splitter" class="splitter" role="separator" aria-label="Resize properties editor" aria-orientation="horizontal" tabindex="0" aria-valuemin="90" aria-valuenow="260"></div><section id="editor-pane"><div class="pane-title"><strong>Object Properties</strong><button id="close-editor" type="button" aria-label="Close properties editor">×</button></div><div id="properties-content" class="muted">Select an object in the tree to inspect its properties.</div></section></main>
<script>
const tree=document.getElementById('tree'), detail=document.getElementById('detail'), status=document.getElementById('status');
let root=null, revision=0, selectedObject=null, token=sessionStorage.getItem('fwb-token')||'';
function headers(){return token?{'Authorization':'Bearer '+token}:{}}
async function api(url,opt={}){let res=await fetch(url,{...opt,headers:{...headers(),...(opt.headers||{})}});if(res.status===401){token=prompt('API bearer token')||'';sessionStorage.setItem('fwb-token',token);return api(url,opt)}if(!res.ok)throw Error(await res.text());return res.json()}
function el(t,txt){const n=document.createElement(t);if(txt!==undefined)n.textContent=txt;return n}
function renderNode(n,parent,depth,query){const matches=((n.name||'')+' '+n.type+' '+(n.id||'')).toLowerCase().includes(query);let children=n.children||[];let matching=children.some(c=>hasMatch(c,query));if(query&&!matches&&!matching)return;
const wrap=el('div');wrap.className='entry';const btn=el('button',(children.length?'▸ ':'  ')+(n.name||n.type)+'  ['+n.type+']');btn.onclick=()=>show(n,btn);wrap.append(btn);parent.append(wrap);
if(depth<2||query){const child=el('div');wrap.append(child);for(const c of children)renderNode(c,child,depth+1,query);btn.firstChild.textContent=btn.textContent.replace('▸','▾')}
else if(children.length){let opened=false;btn.onclick=()=>{show(n,btn);let child=wrap.querySelector('.entry-list');if(opened){child.remove();opened=false;return}child=el('div');child.className='entry-list';wrap.append(child);for(const c of children)renderNode(c,child,depth+1,query);opened=true}}
}
function hasMatch(n,q){return ((n.name||'')+' '+n.type+' '+(n.id||'')).toLowerCase().includes(q)||(n.children||[]).some(c=>hasMatch(c,q))}
function draw(){tree.replaceChildren();if(root)renderNode(root,tree,0,document.getElementById('search').value.toLowerCase())}
async function show(n,button){selectedObject=n;renderProperties(n);document.querySelectorAll('.sel').forEach(x=>x.classList.remove('sel'));if(button)button.classList.add('sel');detail.replaceChildren();detail.append(el('h2',n.name||n.type),el('p',n.type+(n.id?' · '+n.id:'')));
if(n.attributes){detail.append(el('p','Select the properties panel below to inspect object attributes.'))}
if(n.text)detail.append(el('pre',n.text));
if(['Policy','NAT','Routing'].includes(n.type)){try{
const rules=await api('/api/rules?id='+encodeURIComponent(n.id));
const heading=el('div');heading.className='policy-heading';heading.append(el('h3',(n.name||n.type)+' / '+n.type));heading.append(el('span',rules.length+' rules'));detail.append(heading);
const viewport=el('div');viewport.className='policy-scroll';const t=el('table');t.className='policy-table';
const head=el('thead'),hr=el('tr');
for(const name of ['#','Source','Destination','Service','Interface','Direction','Action','Time','Options','Comment','']){const th=el('th',name);hr.append(th)}
head.append(hr);t.append(head);const body=el('tbody');
function objects(rule,keys){const out=[];for(const key of keys)out.push(...(rule.details?.[key]||[]));return out}
function objectCell(row,items){const cell=el('td');cell.className='object-cell';if(!items.length){cell.append(el('span','Any'));row.append(cell);return}
for(const obj of items){const line=el('div');line.className='object-line';const symbol=el('span',objectIcon(obj.type));symbol.className='object-icon';symbol.title=obj.type;line.append(symbol,el('span',obj.name));if(obj.id&&root){line.title=obj.type+' · '+obj.name;line.onclick=ev=>{ev.stopPropagation();const target=findNode(root,obj.id);if(target){selectedObject=target;renderProperties(target)}}}cell.append(line)}row.append(cell)}
rules.forEach((r,i)=>{const row=el('tr');row.className='policy-row';if(r.disabled==='True')row.classList.add('rule-disabled');
row.onclick=()=>{body.querySelectorAll('.rule-selected').forEach(x=>x.classList.remove('rule-selected'));row.classList.add('rule-selected')};
const num=el('td',String(i));num.className='rule-number';row.append(num);
objectCell(row,objects(r,['Src','OSrc','RSrc']));
objectCell(row,objects(r,['Dst','ODst','RDst']));
objectCell(row,objects(r,['Srv','OSrv','TSrv']));
objectCell(row,objects(r,['Itf','Interface','IItf','OItf']));
const direction=el('td');const dir=el('span',(r.direction==='Inbound'?'↓ ':r.direction==='Outbound'?'↑ ':'↕ ')+(r.direction||'Both'));dir.className='direction-label';direction.append(dir);row.append(direction);
const action=el('td');const pill=el('span',(r.action==='Accept'?'✓ ':r.action==='Deny'||r.action==='Reject'?'⊘ ':'• ')+(r.action||r.type));pill.className='action-pill '+(r.action==='Accept'?'accept':r.action==='Deny'||r.action==='Reject'?'deny':'other');action.append(pill);row.append(action);
objectCell(row,objects(r,['When','Time']));
const opts=el('td',r.options?'☑':'');opts.className='option-cell';row.append(opts);
const comment=el('td',r.comment||'');comment.className='comment-cell';row.append(comment);
const controls=el('td');controls.className='rule-controls';if(n.type==='Policy'){const edit=el('button','Edit');edit.onclick=ev=>{ev.stopPropagation();editRule(n,r)};controls.append(edit);
for(const [label,delta] of [['↑',-1],['↓',1]]){const btn=el('button',label);btn.title=delta<0?'Move up':'Move down';btn.onclick=ev=>{ev.stopPropagation();moveRule(n,r,delta)};controls.append(btn)}}row.append(controls);body.append(row)});
t.append(body);viewport.append(t);detail.append(viewport);
}catch(e){detail.append(el('p','Rules: '+e.message))}}

}
async function applyEdit(payload){try{const result=await api('/api/policy/edit',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({...payload,revision})});revision=result.revision;await refresh();return true}catch(e){alert('Edit failed: '+e.message);return false}}
async function moveRule(set,r,offset){if(await applyEdit({set_id:set.id,rule_id:r.id,move:offset}))await show(findNode(root,set.id))}
function objectIcon(type){if(/Service/.test(type))return "⚙";if(/Network|AddressRange/.test(type))return "▣";if(/Firewall|Cluster/.test(type))return "▤";if(/Interface/.test(type))return "▥";if(/Host|IPv4|IPv6|DNSName/.test(type))return "▣";if(/Group/.test(type))return "▦";return "◆"}
function findNode(n,id){if(n.id===id)return n;for(const c of n.children||[]){const x=findNode(c,id);if(x)return x}return null}
async function editRule(set,r){detail.replaceChildren();detail.append(el('h2','Edit policy rule'));const form=el('form');const fields={};
function field(label,value,choices){const wrap=el('p');wrap.append(el('label',label+' '));let control;if(choices){control=el('select');for(const choice of choices){const o=el('option',choice);o.value=choice;control.append(o)}control.value=value}else{control=el('input');control.value=value||'';control.style.width='70%'}wrap.append(control);form.append(wrap);return control}
fields.action=field('Action',r.action,['Accept','Deny','Reject','Continue','Accounting','Return','Branch','Custom','Modify','Pipe','Scrub','Skip']);
fields.direction=field('Direction',r.direction||'Both',['Both','Inbound','Outbound']);
fields.comment=field('Comment',r.comment||'');
fields.disabled=field('Disabled',r.disabled||'False',['False','True']);
const objectTypes={Src:['IPv4','IPv6','Network','NetworkIPv6','Host','Firewall','AddressRange','DNSName','ObjectGroup','AnyNetwork'],Dst:['IPv4','IPv6','Network','NetworkIPv6','Host','Firewall','AddressRange','DNSName','ObjectGroup','AnyNetwork'],Srv:['TCPService','UDPService','IPService','ICMPService','ICMP6Service','ServiceGroup','CustomService','AnyIPService']};
const refs={};for(const [key,types] of Object.entries(objectTypes)){const wrap=el('p');wrap.append(el('label',key+' '));const select=el('select');select.multiple=true;select.size=6;select.style.minWidth='320px';const candidates=[];function collect(n){if(n.id&&types.includes(n.type))candidates.push(n);for(const c of n.children||[])collect(c)}collect(root);for(const obj of candidates){const o=el('option',(obj.name||obj.type)+' ['+obj.type+']');o.value=obj.id;const selected=(r.elements?.[key]||[]).includes(obj.name||obj.type);o.selected=selected;select.append(o)}wrap.append(select);form.append(wrap);refs[key]=select}
const save=el('button','Apply changes');save.type='submit';form.append(save);const cancel=el('button','Cancel');cancel.type='button';cancel.onclick=()=>show(set);form.append(cancel);
form.onsubmit=async ev=>{ev.preventDefault();const payload={set_id:set.id,rule_id:r.id,action:fields.action.value,direction:fields.direction.value,comment:fields.comment.value,disabled:fields.disabled.value,refs:{}};for(const [key,select] of Object.entries(refs))payload.refs[key]=Array.from(select.selectedOptions).map(o=>o.value);if(await applyEdit(payload))await show(findNode(root,set.id))};detail.append(form)}
function renderProperties(n){const p=document.getElementById('properties-content');p.replaceChildren();if(!n)return;const title=el('h3',(n.name||n.type)+' · '+n.type);p.append(title);const table=el('table');for(const [key,value] of Object.entries(n.attributes||{})){const tr=el('tr');tr.append(el('th',key),el('td',value));table.append(tr)}p.append(table);if(n.text)p.append(el('pre',n.text))}
const layoutKey='fwbuilder-web-layout-v1',defaults={left:340,bottom:260,editor:true};
function readLayout(){try{const x=JSON.parse(localStorage.getItem(layoutKey));return {...defaults,...x}}catch{return {...defaults}}}
let layout=readLayout();const leftPane=document.getElementById('left-pane'),upper=document.getElementById('upper'),workspace=document.getElementById('workspace'),editorPane=document.getElementById('editor-pane'),vertical=document.getElementById('vertical-splitter'),horizontal=document.getElementById('horizontal-splitter');
function clamp(v,min,max){return Math.min(Math.max(v,min),Math.max(min,max))}
function applyLayout(){const maxLeft=Math.max(170,upper.clientWidth-190);layout.left=clamp(layout.left,170,maxLeft);leftPane.style.width=layout.left+'px';const maxBottom=Math.max(90,workspace.clientHeight-140);layout.bottom=clamp(layout.bottom,90,maxBottom);editorPane.style.height=layout.bottom+'px';editorPane.hidden=!layout.editor;horizontal.hidden=!layout.editor;document.getElementById('toggle-editor').textContent=layout.editor?'Hide Editor':'Show Editor';vertical.setAttribute('aria-valuenow',String(Math.round(layout.left)));horizontal.setAttribute('aria-valuenow',String(Math.round(layout.bottom)))}
function saveLayout(){localStorage.setItem(layoutKey,JSON.stringify(layout))}
function setEditor(open){layout.editor=open;applyLayout();saveLayout()}
document.getElementById('toggle-editor').onclick=()=>setEditor(!layout.editor);
document.getElementById('close-editor').onclick=()=>setEditor(false);
document.getElementById('reset-layout').onclick=()=>{layout={...defaults};applyLayout();saveLayout()};
function setupSplitter(handle,axis){handle.addEventListener('pointerdown',e=>{if(e.button!==0)return;e.preventDefault();handle.setPointerCapture(e.pointerId);handle.classList.add('dragging');document.body.classList.add(axis==='x'?'resizing':'resizing-y');const start=axis==='x'?e.clientX:e.clientY;const initial=axis==='x'?layout.left:layout.bottom;
const move=ev=>{if(axis==='x')layout.left=initial+ev.clientX-start;else layout.bottom=initial+start-ev.clientY;applyLayout()};
const stop=()=>{handle.removeEventListener('pointermove',move);handle.removeEventListener('pointerup',stop);handle.removeEventListener('pointercancel',stop);handle.classList.remove('dragging');document.body.classList.remove('resizing','resizing-y');saveLayout()};
handle.addEventListener('pointermove',move);handle.addEventListener('pointerup',stop);handle.addEventListener('pointercancel',stop)});
handle.addEventListener('dblclick',()=>{if(axis==='x')layout.left=defaults.left;else layout.bottom=defaults.bottom;applyLayout();saveLayout()});
handle.addEventListener('keydown',e=>{const delta=e.shiftKey?40:10;if(axis==='x'&&(e.key==='ArrowLeft'||e.key==='ArrowRight'))layout.left+=e.key==='ArrowRight'?delta:-delta;else if(axis==='y'&&(e.key==='ArrowUp'||e.key==='ArrowDown'))layout.bottom+=e.key==='ArrowUp'?delta:-delta;else return;e.preventDefault();applyLayout();saveLayout()})}
setupSplitter(vertical,'x');setupSplitter(horizontal,'y');window.addEventListener('resize',applyLayout);applyLayout();
document.getElementById('search').addEventListener('input',draw);
document.getElementById('load').onclick=async()=>{const file=document.getElementById('upload').files[0];if(!file)return;const form=new FormData();form.append('file',file);try{status.textContent='Importing…';const x=await api('/api/import',{method:'POST',body:form});status.textContent=x.filename+' · '+x.count+' nodes';await refresh()}catch(e){status.textContent=e.message}};
async function refresh(){try{const d=await api('/api/document');if(d.loaded){root=d.root;revision=d.revision;if(selectedObject){selectedObject=findNode(root,selectedObject.id)||null;renderProperties(selectedObject)}status.textContent=d.filename+' · '+d.count+' nodes';draw()}else{status.textContent='No document loaded'}}catch(e){status.textContent=e.message}}
refresh();
</script></body></html>`
