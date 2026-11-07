//go:build qz_tci && !ios

package machine

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/majd/ipatool/v2/internal/sap/assets"
	"howett.net/plist"
	"io"
	"math/rand"
	"net/http"
	"os"
	"testing"
	"time"
)

// This opt-in audit freezes only test guest randomness/time. Production shims
// and the app continue to use crypto/rand and the real clock, unchanged.
func TestQingzhouSAPInterpreterDifferential(t *testing.T) {
	jit, tci := os.Getenv("QZ_AUDIT_JIT_LIBRARY"), os.Getenv("QZ_TCI_LIBRARY")
	if jit == "" || tci == "" {
		t.Skip("explicit audit libraries not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	bundle, err := assets.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	type instance struct {
		machine                *Machine
		context                uint64
		randomCalls, timeCalls int
	}
	open := func(path string) *instance {
		t.Setenv("QZ_TCI_LIBRARY", path)
		m, err := Open(ctx, bundle)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = m.Close() })
		state := &instance{machine: m}
		random := rand.New(rand.NewSource(8127301))
		for address, entry := range m.services.entries {
			switch entry.name {
			case "_arc4random":
				entry.handler = func() error { state.randomCalls++; return m.services.setResult(uint64(random.Uint32())) }
			case "_gettimeofday":
				entry.handler = func() error {
					state.timeCalls++
					a, err := m.services.argument(0)
					if err != nil {
						return err
					}
					zone, err := m.services.argument(1)
					if err != nil {
						return err
					}
					if a != 0 {
						var data [16]byte
						binary.LittleEndian.PutUint64(data[:8], uint64(now.Unix()))
						binary.LittleEndian.PutUint32(data[8:12], uint32(now.Nanosecond()/1000))
						if err := m.engine.MemWrite(a, data[:]); err != nil {
							return err
						}
					}
					if zone != 0 {
						if err := m.engine.MemWrite(zone, make([]byte, 8)); err != nil {
							return err
						}
					}
					return m.services.setResult(0)
				}
			}
			m.services.entries[address] = entry
		}
		state.context, err = m.Initialize([]byte{2, 0, 0, 0, 0, 1})
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	a, b := open(jit), open(tci)
	client := &http.Client{Timeout: 30 * time.Second}
	fetch := func(url, key string, body []byte) []byte {
		method := "GET"
		if body != nil {
			method = "POST"
		}
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("User-Agent", "Configurator/2.17 (Macintosh; OS X 15.2; 24C5089c) AppleWebKit/0620.1.16.11.6")
		if body != nil {
			req.Header.Set("Content-Type", "application/x-plist")
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 {
			t.Fatalf("anonymous SAP setup HTTP %d", res.StatusCode)
		}
		var values map[string]any
		if _, err := plist.Unmarshal(data, &values); err != nil {
			t.Fatal(err)
		}
		value, ok := values[key].([]byte)
		if !ok || len(value) == 0 {
			t.Fatal("missing anonymous SAP setup buffer")
		}
		return value
	}
	certificate := fetch("https://s.mzstatic.com/sap/setupCert.plist", "sign-sap-setup-cert", nil)
	hardware := []byte{2, 0, 0, 0, 0, 1}
	exchange := func(s *instance, input []byte) ([]byte, int32) {
		output, status, err := s.machine.Exchange(200, hardware, s.context, input)
		if err != nil {
			t.Fatal(err)
		}
		return output, status
	}
	firstA, statusA := exchange(a, certificate)
	firstB, statusB := exchange(b, certificate)
	if statusA != 1 || statusB != 1 {
		t.Fatalf("setup stage differs: %d/%d", statusA, statusB)
	}
	if !bytes.Equal(firstA, firstB) {
		t.Fatalf("SETUP_MISMATCH: random calls %d/%d; time calls %d/%d; lengths %d/%d", a.randomCalls, b.randomCalls, a.timeCalls, b.timeCalls, len(firstA), len(firstB))
	}
	t.Log("MATCH: initial SAP setup request")
	envelope, err := plist.Marshal(map[string]any{"sign-sap-setup-buffer": firstA}, plist.XMLFormat)
	if err != nil {
		t.Fatal(err)
	}
	reply := fetch("https://fpinit.itunes.apple.com/v1/signSapSetup/legacy", "sign-sap-setup-buffer", envelope)
	_, statusA = exchange(a, reply)
	_, statusB = exchange(b, reply)
	if statusA != 0 || statusB != 0 {
		t.Fatalf("setup completion differs: %d/%d", statusA, statusB)
	}
	for i, input := range [][]byte{[]byte("Qingzhou anonymous differential probe"), []byte("second probe"), bytes.Repeat([]byte("nonsecret-payload"), 256)} {
		sigA, err := a.machine.Sign(a.context, input)
		if err != nil {
			t.Fatal(err)
		}
		sigB, err := b.machine.Sign(b.context, input)
		if err != nil {
			t.Fatal(err)
		}
		if len(sigA) == 0 || !bytes.Equal(sigA, sigB) {
			t.Fatal(fmt.Sprintf("SIGNATURE_MISMATCH: probe %d; random calls %d/%d; time calls %d/%d; lengths %d/%d", i, a.randomCalls, b.randomCalls, a.timeCalls, b.timeCalls, len(sigA), len(sigB)))
		}
		t.Logf("MATCH: signature probe %d (%d bytes)", i, len(sigA))
	}
	if a.randomCalls != b.randomCalls || a.timeCalls != b.timeCalls {
		t.Fatal("guest nondeterministic call counts differ")
	}
	t.Logf("AUDIT_PASS: official JIT and pinned TCI agree; random calls=%d, time calls=%d", a.randomCalls, a.timeCalls)
}
