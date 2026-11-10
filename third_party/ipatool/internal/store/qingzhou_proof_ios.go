package store

/*
#include <stdlib.h>
char *QZStoreProof(const char *requestJSON);
*/
import "C"
import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unsafe"
)

func platformMachineProof(ctx context.Context, action, challenge, guid string) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	input, err := json.Marshal(map[string]string{"action": action, "challenge": challenge, "guid": guid})
	if err != nil {
		return nil, err
	}
	ptr := C.CString(string(input))
	defer C.free(unsafe.Pointer(ptr))
	result := C.QZStoreProof(ptr)
	if result == nil {
		return nil, errors.New("Store device verification unavailable")
	}
	defer C.free(unsafe.Pointer(result))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var decoded struct {
		Headers map[string]string `json:"headers"`
		Error   string            `json:"error"`
	}
	if json.Unmarshal([]byte(C.GoString(result)), &decoded) != nil || decoded.Error != "" {
		return nil, errors.New("Store device verification failed (STORE_PROOF)")
	}
	headers := map[string]string{}
	for _, name := range []string{"X-Apple-AMD", "X-Apple-AMD-M"} {
		if v := decoded.Headers[name]; v != "" {
			if len(v) > 16384 || strings.ContainsAny(v, "\r\n") {
				return nil, errors.New("invalid Store device proof")
			}
			headers[name] = v
		}
	}
	return headers, nil
}
