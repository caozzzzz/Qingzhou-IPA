"""Run on macOS: tests the exact URLSession adapter with local fixtures only."""
import base64, gzip, http.server, json, pathlib, ssl, subprocess, tempfile, threading
root=pathlib.Path(__file__).resolve().parents[2]
calls=[]
class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self,*args): pass
    def do_GET(self):
        calls.append((self.path,b"",None,None,self.command))
        body=b'{"trustedPhoneNumbers":[]}'
        self.send_response(200)
        self.send_header("Content-Type","application/json")
        self.send_header("Content-Length",str(len(body)))
        self.end_headers();self.wfile.write(body)
    def do_PUT(self):
        self.do_POST()
    def do_POST(self):
        body=self.rfile.read(int(self.headers.get("Content-Length",0)))
        calls.append((self.path,body,self.headers.get("X-Apple-ActionSignature"),self.headers.get("Cookie"),self.command))
        if self.path in ["/redirect","/missing"]:
            self.send_response(302)
            if self.path=="/redirect": self.send_header("location","/must-not-follow")
            self.send_header("Content-Length","0");self.end_headers();return
        self.send_response(200)
        if self.path=="/gzip":
            compressed=gzip.compress(body)
            self.send_header("Content-Encoding","gzip")
            self.send_header("Content-Length",str(len(compressed)))
            self.end_headers();self.wfile.write(compressed);return
        self.send_header("Set-Cookie","first=one; Path=/; HttpOnly")
        self.send_header("Set-Cookie","second=two; Path=/; Expires=Wed, 01 Jan 2031 00:00:00 GMT")
        self.send_header("Content-Length",str(len(body)));self.end_headers();self.wfile.write(body)
server=http.server.ThreadingHTTPServer(("127.0.0.1",0),Handler)
threading.Thread(target=server.serve_forever,daemon=True).start()
with tempfile.TemporaryDirectory() as temp:
    temp=pathlib.Path(temp);binary=temp/"native-http-test"
    subprocess.run(["clang","-fobjc-arc","-framework","Foundation",str(root/"ios/NativeHTTP.m"),str(root/"ios/tests/NativeHTTPHarness.m"),"-o",str(binary)],check=True)
    def run(path,headers=None,url=None,method="POST"):
        body=b"" if method=="GET" else b"fixture-only-body"
        payload={"url":url or f"http://127.0.0.1:{server.server_port}{path}","method":method,"headers":headers or {"X-Apple-ActionSignature":["fixture-signature"]},"body":base64.b64encode(body).decode(),"timeout":5}
        completed=subprocess.run([str(binary)],input=json.dumps(payload),text=True,capture_output=True,check=True,timeout=15)
        return json.loads(completed.stdout)
    result=run("/redirect");assert result["status"]==302
    assert next(v for k,v in result["headers"].items() if k.lower()=="location")==["/must-not-follow"]
    assert len(calls)==1 and calls[0][0]=="/redirect"
    result=run("/missing");assert result["status"]==302
    assert not any(k.lower()=="location" for k in result["headers"])
    result=run("/echo");assert result["status"]==200 and base64.b64decode(result["body"])==b"fixture-only-body"
    assert {c["Name"] for c in result["cookies"]}=={"first","second"}
    assert next(c for c in result["cookies"] if c["Name"]=="first")["HTTPOnly"]
    assert calls[-1][2]=="fixture-signature" and calls[-1][3] is None
    run("/echo",{"Cookie":["explicit=from-go"]});assert calls[-1][3]=="explicit=from-go"
    run("/echo");assert calls[-1][3] is None
    result=run("/gzip");assert result["status"]==200 and base64.b64decode(result["body"])==b"fixture-only-body"
    result=run("/auth",method="GET");assert result["status"]==200 and calls[-1][4]=="GET"
    result=run("/auth/verify/phone",method="PUT");assert result["status"]==200 and calls[-1][4]=="PUT" and calls[-1][1]==b"fixture-only-body"
    # An untrusted TLS peer must fail; no trust callback/bypass is installed.
    key=temp/"key.pem";cert=temp/"cert.pem"
    subprocess.run(["openssl","req","-x509","-newkey","rsa:2048","-nodes","-keyout",str(key),"-out",str(cert),"-days","1","-subj","/CN=localhost"],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    tls=http.server.ThreadingHTTPServer(("127.0.0.1",0),Handler)
    context=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);context.load_cert_chain(str(cert),str(key));tls.socket=context.wrap_socket(tls.socket,server_side=True)
    threading.Thread(target=tls.serve_forever,daemon=True).start()
    result=run("/echo",url=f"https://127.0.0.1:{tls.server_port}/echo")
    assert result.get("errorCode",0)!=0 and "status" not in result
    assert result["errorDomain"] in {"NSURLErrorDomain", "NSPOSIXErrorDomain", "NSOSStatusErrorDomain", "kCFErrorDomainCFNetwork", "other"}
    tls.shutdown()
server.shutdown()
print("URLSession fixtures passed: signed POST/body, redirect interception, missing Location, cookies, session isolation, gzip decoding, TLS rejection")
