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
input{background:#0f172a;color:#e5e7eb;border:1px solid #475569;border-radius:6px;padding:8px}main{display:grid;grid-template-columns:340px minmax(0,1fr);height:calc(100vh - 68px)}
aside{overflow:auto;border-right:1px solid #334155;padding:12px}article{padding:24px;overflow:auto}#tree button{background:none;text-align:left;width:100%;color:#dbeafe;padding:4px;border-radius:4px}
#tree button:hover,#tree button.sel{background:#334155}.entry{margin-left:14px;border-left:1px solid #334155;padding-left:7px}
small,.muted{color:#94a3b8}table{border-collapse:collapse;width:100%;margin-top:16px}th,td{border-bottom:1px solid #334155;padding:10px;text-align:left;vertical-align:top}th{color:#93c5fd}pre{white-space:pre-wrap;overflow-wrap:anywhere;background:#172235;padding:12px;border-radius:6px}.toolbar{display:flex;gap:8px;align-items:center;flex-wrap:wrap}
@media(max-width:800px){main{grid-template-columns:1fr;height:auto}aside{max-height:45vh;border-right:0;border-bottom:1px solid #334155}}
</style></head><body><header><h1>🛡 fwbuilder-web</h1><span class="muted">Go • Policy editor (in-memory)</span><input type="file" id="upload" accept=".fwb,.xml"><button id="load">Import .fwb</button><span id="status"></span></header>
<main><aside><div class="toolbar"><input id="search" placeholder="Search objects…" aria-label="Search objects" style="width:100%"></div><div id="tree"></div></aside><article id="detail"><h2>Firewall Builder</h2><p class="muted">Import an existing .fwb file to browse its objects and rule sets.</p></article></main>
<script>
const tree=document.getElementById('tree'), detail=document.getElementById('detail'), status=document.getElementById('status');
let root=null, revision=0, token=sessionStorage.getItem('fwb-token')||'';
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
async function show(n,button){document.querySelectorAll('.sel').forEach(x=>x.classList.remove('sel'));if(button)button.classList.add('sel');detail.replaceChildren();detail.append(el('h2',n.name||n.type),el('p',n.type+(n.id?' · '+n.id:'')));
if(n.attributes){const dl=el('table');for(const [k,v] of Object.entries(n.attributes)){const tr=el('tr');tr.append(el('th',k),el('td',v));dl.append(tr)}detail.append(dl)}
if(n.text)detail.append(el('pre',n.text));
if(['Policy','NAT','Routing'].includes(n.type)){try{const rules=await api('/api/rules?id='+encodeURIComponent(n.id));detail.append(el('h3','Rules ('+rules.length+')'));const t=el('table');let tr=el('tr');for(const k of ['#','Action','Direction','Source','Destination','Service','Comment','Edit'])tr.append(el('th',k));t.append(tr);rules.forEach((r,i)=>{const row=el('tr');const e=r.elements||{};const pick=(...names)=>names.flatMap(k=>e[k]||[]).join(', ')||'Any';for(const v of [String(i+1)+(r.disabled==='True'?' (disabled)':''),r.action||r.type,r.direction||'',pick('Src','OSrc','RSrc'),pick('Dst','ODst','RDst'),pick('Srv','OSrv','TSrv'),r.comment||''])row.append(el('td',v));const cell=el('td');if(n.type==='Policy'){const edit=el('button','Edit');edit.onclick=()=>editRule(n,r);cell.append(edit);const up=el('button','↑');up.title='Move rule up';up.onclick=()=>moveRule(n,r,-1);cell.append(up);const down=el('button','↓');down.title='Move rule down';down.onclick=()=>moveRule(n,r,1);cell.append(down)}row.append(cell);t.append(row)});detail.append(t)}catch(e){detail.append(el('p','Rules: '+e.message))}}
}
async function applyEdit(payload){try{const result=await api('/api/policy/edit',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({...payload,revision})});revision=result.revision;await refresh();return true}catch(e){alert('Edit failed: '+e.message);return false}}
async function moveRule(set,r,offset){if(await applyEdit({set_id:set.id,rule_id:r.id,move:offset}))await show(findNode(root,set.id))}
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
document.getElementById('search').addEventListener('input',draw);
document.getElementById('load').onclick=async()=>{const file=document.getElementById('upload').files[0];if(!file)return;const form=new FormData();form.append('file',file);try{status.textContent='Importing…';const x=await api('/api/import',{method:'POST',body:form});status.textContent=x.filename+' · '+x.count+' nodes';await refresh()}catch(e){status.textContent=e.message}};
async function refresh(){try{const d=await api('/api/document');if(d.loaded){root=d.root;revision=d.revision;status.textContent=d.filename+' · '+d.count+' nodes';draw()}else{status.textContent='No document loaded'}}catch(e){status.textContent=e.message}}
refresh();
</script></body></html>`
