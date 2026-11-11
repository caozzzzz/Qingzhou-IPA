package smsauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type identity struct {
	Identifier []byte `json:"identifier"`
	Provision  []byte `json:"adi_pb"`
}

func (id identity) valid() bool {
	return len(id.Identifier) == 16 && len(id.Provision) > 0 && len(id.Provision) <= 256<<10
}

func (id identity) deviceHeaders() map[string]string {
	value := hex.EncodeToString(id.Identifier)
	return map[string]string{
		"X-Apple-I-MD-LU": hex.EncodeToString(sha(id.Identifier)),
		"X-Mme-Device-Id": value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:],
	}
}

func (c *Client) cachePath() string {
	if !filepath.IsAbs(c.cacheDir) {
		return ""
	}
	return filepath.Join(c.cacheDir, "qingzhou", "sms-device-v1.json")
}

func (c *Client) readIdentity() identity {
	var id identity
	file, err := os.Open(c.cachePath())
	if err != nil {
		return id
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (512<<10)+1))
	if err != nil || len(data) > 512<<10 || json.Unmarshal(data, &id) != nil || !id.valid() {
		return identity{}
	}
	return id
}

func (c *Client) saveIdentity() {
	path := c.cachePath()
	if path == "" || os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return
	}
	data, err := json.Marshal(c.identity)
	if err != nil {
		return
	}
	defer clear(data)
	file, err := os.CreateTemp(filepath.Dir(path), "sms-device-")
	if err != nil {
		return
	}
	defer os.Remove(file.Name())
	_, err = file.Write(data)
	closeErr := file.Close()
	if err == nil && closeErr == nil {
		_ = os.Rename(file.Name(), path)
	}
}

func decodeProofData(value string, maximum int) ([]byte, bool) {
	if value == "" || len(value) > base64.StdEncoding.EncodedLen(maximum) {
		return nil, false
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	return decoded, err == nil && len(decoded) > 0 && len(decoded) <= maximum && base64.StdEncoding.EncodeToString(decoded) == value
}

// Provisioning messages contain only an app-generated device identity and
// Apple provisioning material. Account credentials never enter this client.
func (c *Client) provision(ctx context.Context) (identity, error) {
	const stage = "SMS_DEVICE_PROVISION"
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	id := identity{Identifier: make([]byte, 16)}
	if _, err := rand.Read(id.Identifier); err != nil {
		return identity{}, &Error{Stage: stage}
	}
	conn, _, err := websocket.Dial(ctx, "wss://"+proofHost+"/v3/provisioning_session", &websocket.DialOptions{HTTPClient: c.device})
	if err != nil {
		return identity{}, &Error{Stage: stage}
	}
	defer conn.CloseNow()
	conn.SetReadLimit(512 << 10)
	steps := []string{"GiveIdentifier", "GiveStartProvisioningData", "GiveEndProvisioningData", "ProvisioningSuccess"}
	for _, step := range steps {
		var message struct {
			Result    string `json:"result"`
			Client    string `json:"cpim"`
			Provision string `json:"adi_pb"`
		}
		if wsjson.Read(ctx, conn, &message) != nil || message.Result != step {
			return identity{}, &Error{Stage: stage}
		}
		var reply map[string]string
		switch step {
		case "GiveIdentifier":
			reply = map[string]string{"identifier": base64.StdEncoding.EncodeToString(id.Identifier)}
		case "GiveStartProvisioningData":
			var start struct {
				Status gsaStatus `plist:"Status"`
				Data   string    `plist:"spim"`
			}
			err := c.applePlist(ctx, c.endpoints.Start, stage, map[string]any{}, map[string]any{}, id.deviceHeaders(), &start)
			if err != nil {
				return identity{}, err
			}
			if _, ok := decodeProofData(start.Data, 64<<10); !ok || start.Status.Code != 0 {
				return identity{}, &Error{Stage: stage, Code: start.Status.Code}
			}
			reply = map[string]string{"spim": start.Data}
		case "GiveEndProvisioningData":
			if _, ok := decodeProofData(message.Client, 64<<10); !ok {
				return identity{}, &Error{Stage: stage}
			}
			var finish struct {
				Status gsaStatus `plist:"Status"`
				PTM    string    `plist:"ptm"`
				TK     string    `plist:"tk"`
			}
			err := c.applePlist(ctx, c.endpoints.Finish, stage, map[string]any{}, map[string]any{"cpim": message.Client}, id.deviceHeaders(), &finish)
			if err != nil {
				return identity{}, err
			}
			_, ptmOK := decodeProofData(finish.PTM, 64<<10)
			_, tkOK := decodeProofData(finish.TK, 64<<10)
			if !ptmOK || !tkOK || finish.Status.Code != 0 {
				return identity{}, &Error{Stage: stage, Code: finish.Status.Code}
			}
			reply = map[string]string{"ptm": finish.PTM, "tk": finish.TK}
		case "ProvisioningSuccess":
			data, ok := decodeProofData(message.Provision, 256<<10)
			if !ok {
				return identity{}, &Error{Stage: stage}
			}
			id.Provision = data
			return id, nil
		}
		if wsjson.Write(ctx, conn, reply) != nil {
			return identity{}, &Error{Stage: stage}
		}
	}
	return identity{}, &Error{Stage: stage}
}

func (c *Client) proofHeaders(ctx context.Context) (map[string]string, error) {
	if !c.identity.valid() {
		c.identity = c.readIdentity()
	}
	if c.identity.valid() {
		headers, err := c.fetchProofHeaders(ctx)
		if !errorIsStage(err, "SMS_DEVICE_EXPIRED") {
			return headers, err
		}
	}
	id, err := c.provision(ctx)
	if err != nil {
		return nil, err
	}
	c.identity = id
	headers, err := c.fetchProofHeaders(ctx)
	if err == nil {
		c.saveIdentity()
	}
	return headers, err
}

func (c *Client) fetchProofHeaders(ctx context.Context) (map[string]string, error) {
	const stage = "SMS_DEVICE_PROOF"
	status, raw, err := jsonRequest(ctx, c.device, http.MethodPost, "https://"+proofHost+"/v3/get_headers", proofHost, stage, c.identity, nil)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var reply struct {
		Result  string          `json:"result"`
		Message string          `json:"message"`
		OTP     string          `json:"X-Apple-I-MD"`
		Machine string          `json:"X-Apple-I-MD-M"`
		Routing json.RawMessage `json:"X-Apple-I-MD-RINFO"`
	}
	if json.Unmarshal(raw, &reply) != nil {
		return nil, &Error{Stage: stage, HTTP: status}
	}
	if reply.Result != "Headers" && strings.Contains(reply.Message, "-45061") {
		return nil, &Error{Stage: "SMS_DEVICE_EXPIRED", HTTP: status}
	}
	_, otpOK := decodeProofData(reply.OTP, 4096)
	_, machineOK := decodeProofData(reply.Machine, 4096)
	if status != http.StatusOK || reply.Result != "Headers" || !otpOK || !machineOK {
		return nil, &Error{Stage: stage, HTTP: status}
	}
	routing := strings.Trim(string(reply.Routing), "\"")
	if _, err := strconv.ParseUint(routing, 10, 64); err != nil {
		routing = "0"
	}
	headers := c.identity.deviceHeaders()
	headers["X-Apple-I-MD"] = reply.OTP
	headers["X-Apple-I-MD-M"] = reply.Machine
	headers["X-Apple-I-MD-RINFO"] = routing
	return headers, nil
}
