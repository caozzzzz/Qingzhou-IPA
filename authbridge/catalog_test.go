package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)
func TestCatalogSearchIDAndNameMetadata(t *testing.T){
	var queries []string
	s:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		queries=append(queries,r.URL.String())
		if r.URL.Query().Get("country")!="cn"{t.Error("country not forwarded")}
		w.Header().Set("Content-Type","application/json")
		_,_=w.Write([]byte(`{"results":[{"trackId":414478124,"trackName":"微信","bundleId":"com.tencent.xin","version":"8.0","currentVersionReleaseDate":"2026-10-01T00:00:00Z","fileSizeBytes":"123456789","supportedDevices":["iPhone14","iPad8"],"kind":"software"},{"trackId":42,"trackName":"Pad only","supportedDevices":["iPad8"]}]}`))
	}));defer s.Close()
	old:=catalogEndpoint;catalogEndpoint=s.URL;defer func(){catalogEndpoint=old}()
	apps,err:=catalogSearch("414478124","cn","iphone");if err!=nil||len(apps)!=1{t.Fatalf("lookup failed: %v %v",apps,err)}
	if apps[0].SizeBytes!=123456789||apps[0].UpdatedAt!="2026-10-01T00:00:00Z"{t.Fatal("metadata lost")}
	apps,err=catalogSearch("微信","cn","ipad");if err!=nil||len(apps)!=2{t.Fatal("iPad search failed")}
	if len(queries)!=2||queries[0][:7]!="/lookup"||queries[1][:7]!="/search"{t.Fatal("ID/name routing incorrect")}
	if _,err=catalogSearch("x","bad","iphone");err==nil{t.Fatal("invalid country accepted")}
}
