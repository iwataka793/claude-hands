import json,subprocess,sys,os,time,threading
calls=json.loads(sys.argv[1])
msgs=[{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}},{"jsonrpc":"2.0","method":"notifications/initialized"}]
for i,(n,a) in enumerate(calls):
    msgs.append({"jsonrpc":"2.0","id":10+i,"method":"tools/call","params":{"name":n,"arguments":a}})
p=subprocess.Popen([os.environ.get("PCMCP","/tmp/pc-linux")],stdin=subprocess.PIPE,stdout=subprocess.PIPE)
for m in msgs: p.stdin.write((json.dumps(m,ensure_ascii=False)+"\n").encode()); p.stdin.flush()
got={}
t0=time.time()
while len(got)<len(calls) and time.time()-t0<180:
    l=p.stdout.readline()
    if not l: break
    m=json.loads(l)
    if m.get("id",0)>=10: got[m["id"]]=m
p.stdin.close(); p.kill()
for i in sorted(got):
    r=got[i].get("result",{}); 
    if "error" in got[i]: print("RPCERR",got[i]["error"])
    t=[c.get("text","<画像>") if c.get("type")=="text" else "<画像>" for c in r.get("content",[])]
    print(f"--- {calls[i-10][0]} err={r.get('isError')}\n"+"\n".join(t)[:1200])
