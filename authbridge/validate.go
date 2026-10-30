package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"strings"

	"howett.net/plist"
)
func validateIPA(file,platform,versionID string,appID int64)error{
	reader,err:=zip.OpenReader(file);if err!=nil{return err};defer reader.Close()
	found:=false
	for _,f:=range reader.File{
		parts:=strings.Split(f.Name,"/")
		isInfo:=len(parts)==3&&parts[0]=="Payload"&&strings.HasSuffix(parts[1],".app")&&parts[2]=="Info.plist"
		isMetadata:=f.Name=="iTunesMetadata.plist"
		if !isInfo&&!isMetadata{continue}
		r,e:=f.Open();if e!=nil{return e};data,e:=io.ReadAll(io.LimitReader(r,8<<20));r.Close();if e!=nil{return e}
		if isInfo{
			var info struct{Families []int `plist:"UIDeviceFamily"`;Version string `plist:"CFBundleShortVersionString"`}
			if _,e=plist.Unmarshal(data,&info);e!=nil{return e};found=true
			expected:=1;if platform=="ipad"{expected=2}
			compatible:=false;for _,family:=range info.Families{if family==expected{compatible=true}}
			if !compatible&&(len(info.Families)>0||platform=="ipad"){return errors.New("device family mismatch")}
		}
		if isMetadata{
			var metadata map[string]interface{};if _,e=plist.Unmarshal(data,&metadata);e!=nil{return e}
			if id,ok:=metadata["softwareVersionExternalIdentifier"];ok&&fmt.Sprint(id)!=versionID{return errors.New("version identifier mismatch")}
			if id,ok:=metadata["itemId"];ok&&fmt.Sprint(id)!=fmt.Sprint(appID){return errors.New("app identifier mismatch")}
		}
	}
	if !found{return errors.New("missing application metadata")};return nil
}
