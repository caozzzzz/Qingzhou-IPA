//go:build ios

package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"unsafe"
)

type iosClient struct {
	operation sync.Mutex
	state     sync.Mutex
	backend   bridge
	cancel    context.CancelFunc
	progress  []byte
}

var iosClients sync.Map
var iosNextID atomic.Uint64

//export QZSetCacheDirectory
func QZSetCacheDirectory(path *C.char) C.int {
	root := C.GoString(path)
	if !filepath.IsAbs(root) {
		return 0
	}
	if os.MkdirAll(root, 0700) != nil {
		return 0
	}
	if os.Setenv("QINGZHOU_CACHE_DIR", root) != nil {
		return 0
	}
	return 1
}

//export QZSetSAPImportDirectory
func QZSetSAPImportDirectory(path *C.char) {
	root := C.GoString(path)
	if filepath.IsAbs(root) {
		_ = os.Setenv("QINGZHOU_SAP_IMPORT_DIR", root)
	}
}

//export QZCreate
func QZCreate() C.ulonglong {
	c := &iosClient{}
	c.backend.emit = func(r response) { data, _ := json.Marshal(r); c.state.Lock(); c.progress = data; c.state.Unlock() }
	id := iosNextID.Add(1)
	iosClients.Store(id, c)
	return C.ulonglong(id)
}

//export QZCall
func QZCall(handle C.ulonglong, input *C.char) (output *C.char) {
	defer func() {
		if recover() != nil {
			output = C.CString(`{"ok":false,"error":"认证服务异常，请重新打开应用后重试。"}`)
		}
	}()
	value, ok := iosClients.Load(uint64(handle))
	if !ok {
		return C.CString(`{"ok":false,"error":"会话已关闭。"}`)
	}
	c := value.(*iosClient)
	c.operation.Lock()
	defer c.operation.Unlock()
	var in request
	raw := C.GoString(input)
	if len(raw) > 8<<20 || json.Unmarshal([]byte(raw), &in) != nil {
		return C.CString(`{"ok":false,"error":"请求格式无效。"}`)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in.Context = ctx
	c.state.Lock()
	c.cancel = cancel
	c.progress = nil
	c.state.Unlock()
	defer func() { c.state.Lock(); c.cancel = nil; c.state.Unlock() }()
	result := c.backend.handle(in)
	in.Password = ""
	encoded, err := json.Marshal(result)
	if err != nil {
		return C.CString(`{"ok":false,"error":"响应格式无效。"}`)
	}
	return C.CString(string(encoded))
}

//export QZPoll
func QZPoll(handle C.ulonglong) *C.char {
	value, ok := iosClients.Load(uint64(handle))
	if !ok {
		return C.CString(`{}`)
	}
	c := value.(*iosClient)
	c.state.Lock()
	defer c.state.Unlock()
	if len(c.progress) == 0 {
		return C.CString(`{}`)
	}
	return C.CString(string(c.progress))
}

//export QZCancel
func QZCancel(handle C.ulonglong) {
	if value, ok := iosClients.Load(uint64(handle)); ok {
		c := value.(*iosClient)
		c.state.Lock()
		defer c.state.Unlock()
		if c.cancel != nil {
			c.cancel()
		}
	}
}

//export QZClose
func QZClose(handle C.ulonglong) { QZCancel(handle); iosClients.Delete(uint64(handle)) }

//export QZFree
func QZFree(pointer *C.char) { C.free(unsafe.Pointer(pointer)) }
