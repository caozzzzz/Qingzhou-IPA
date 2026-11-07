"""Fetch the exact upstream macOS ARM64 Unicorn runtime for opt-in audit."""
import hashlib,io,pathlib,urllib.request,zipfile
url="https://files.pythonhosted.org/packages/6c/ae/4943c6f8524d729ec7d5e69df6407ea05d710fe77471d91cecf3fc64eb57/unicorn-2.1.4-cp37-abi3-macosx_11_0_arm64.whl"
data=urllib.request.urlopen(url,timeout=60).read()
assert hashlib.sha256(data).hexdigest()=="d6c93e0f60328d8f4a1792af3f834137a28050fcc2305f2ec01efe8558a9844e"
library=zipfile.ZipFile(io.BytesIO(data)).read("unicorn/lib/libunicorn.2.dylib")
assert hashlib.sha256(library).hexdigest()=="7207c8e3d7a63118fb0bca73e01816797fd51b1d8a39a4cbc7abfd562ee59c85"
path=pathlib.Path("ios/build/unicorn-audit-jit/libunicorn.2.dylib");path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(library)
print("Official audit runtime archive and library digests verified")
