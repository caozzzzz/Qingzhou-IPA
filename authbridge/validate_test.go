package main

import("archive/zip";"os";"path/filepath";"testing";"howett.net/plist")
func TestIPADeviceAndVersionValidation(t *testing.T){
	p:=filepath.Join(t.TempDir(),"fixture.ipa");f,_:=os.Create(p);z:=zip.NewWriter(f)
	info,_:=plist.Marshal(map[string]interface{}{"UIDeviceFamily":[]int{1,2},"CFBundleShortVersionString":"1.0"},plist.BinaryFormat)
	w,_:=z.Create("Payload/Test.app/Info.plist");_,_=w.Write(info)
	m,_:=plist.Marshal(map[string]interface{}{"itemId":int64(123),"softwareVersionExternalIdentifier":int64(456)},plist.BinaryFormat)
	w,_=z.Create("iTunesMetadata.plist");_,_=w.Write(m);z.Close();f.Close()
	if err:=validateIPA(p,"ipad","456",123);err!=nil{t.Fatal(err)}
	if err:=validateIPA(p,"iphone","456",123);err!=nil{t.Fatal(err)}
	if err:=validateIPA(p,"ipad","789",123);err==nil{t.Fatal("wrong version accepted")}
	if err:=validateIPA(p,"ipad","456",999);err==nil{t.Fatal("wrong app accepted")}
}
