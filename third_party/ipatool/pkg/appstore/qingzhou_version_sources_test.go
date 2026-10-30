package appstore

import (
	"github.com/majd/ipatool/v2/pkg/http"
	"testing"
)

func TestQingzhouVersionNormalization(t *testing.T) {
	for _, v := range []interface{}{float64(893607403), int64(893607403), uint64(893607403), "893607403"} {
		s, e := normalizeVersionIdentifier(v)
		if e != nil || s != "893607403" {
			t.Fatalf("normalization failed: %v %v", s, e)
		}
	}
	for _, v := range []interface{}{float64(1.5), "8.9e+08", nil, "bad"} {
		if _, e := normalizeVersionIdentifier(v); e == nil {
			t.Fatal("invalid identifier accepted")
		}
	}
	res := http.Result[downloadResult]{Data: downloadResult{Items: []downloadItemResult{{Metadata: map[string]interface{}{"softwareVersionExternalIdentifiers": []interface{}{float64(893607403), int64(893607402), float64(893607403)}, "softwareVersionExternalIdentifier": float64(893607403)}}}}}
	result, e := parseVersionListResponse(res)
	if e != nil || len(result.ExternalVersionIdentifiers) != 2 || result.LatestExternalVersionID != "893607403" {
		t.Fatalf("list parse failed: %+v %v", result, e)
	}
}
