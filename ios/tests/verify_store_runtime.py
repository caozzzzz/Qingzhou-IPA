from pathlib import Path
import json,hashlib
root=Path(__file__).resolve().parents[2]/'third_party/ipadn-store-runtime'
manifest=json.loads((root/'manifest.json').read_text())
for name,digest in manifest['files'].items():
    assert Path(name).name==name
    assert hashlib.sha256((root/'files'/name).read_bytes()).hexdigest()==digest,name
assert "connect-src 'self'" in (root/'files/index.html').read_text()
print('Store runtime integrity and same-origin policy verified')
