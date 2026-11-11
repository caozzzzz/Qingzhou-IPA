package smsauth

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"

	"howett.net/plist"
)

// Apple GrandSlam uses SRP-6a with RFC 5054's 2048-bit group and the
// s2k/s2k_fo password transform. See the protocol references in ios/README.md.
var srpGroup, _ = new(big.Int).SetString(
	"ac6bdb41324a9a9bf166de5e1389582faf72b6651987ee07fc3192943db56050"+
		"a37329cbb4a099ed8193e0757767a13dd52312ab4b03310dcd7f48a9da04fd50"+
		"e8083969edb767b0cf6095179a163ab3661a05fbd5faaae82918a9962f0b93b8"+
		"55f97993ec975eeaa80d740adbf4ff747359d041d5c33ea71d281e446b14773b"+
		"ca97b43a23fb801676bd207a436c6481f1d2b9078717461a5b9d32e688f87748"+
		"544523b524b0d57d5ea77a2775d2ecfa032cfbdbf52fb3786160279004e57ae6"+
		"af874e7303ce53299ccc041c7bc308d82a5698f3a8d0c38271ae35f8e9dbfbb6"+
		"94b5c803d89f7ae435de236d525f54759b65e372fcd68ef20fa7111f9e4aff73", 16)

var srpGenerator = big.NewInt(2)

type srpClient struct {
	private, public *big.Int
}

func newSRPClient() (*srpClient, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, err
	}
	secret[0] |= 0x80
	return srpFromSecret(secret[:]), nil
}

func srpFromSecret(secret []byte) *srpClient {
	private := new(big.Int).SetBytes(secret)
	return &srpClient{private: private, public: new(big.Int).Exp(srpGenerator, private, srpGroup)}
}

func sha(chunks ...[]byte) []byte {
	hash := sha256.New()
	for _, chunk := range chunks {
		hash.Write(chunk)
	}
	return hash.Sum(nil)
}

type srpProof struct {
	client, server, key, premaster []byte
}

func (s *srpClient) proof(email, password, protocol string, salt, serverPublic []byte, iterations int) (srpProof, error) {
	failure := &Error{Stage: "SMS_SRP"}
	if len(salt) < 8 || len(salt) > 64 || len(serverPublic) == 0 || len(serverPublic) > 256 || iterations < 1 || iterations > 500_000 {
		return srpProof{}, failure
	}
	server := new(big.Int).SetBytes(serverPublic)
	if server.Sign() <= 0 || server.Cmp(srpGroup) >= 0 {
		return srpProof{}, failure
	}
	prehash := sha([]byte(password))
	switch protocol {
	case "s2k":
	case "s2k_fo":
		prehash = []byte(hex.EncodeToString(prehash))
	default:
		return srpProof{}, failure
	}
	passwordKey, err := pbkdf2.Key(sha256.New, string(prehash), salt, iterations, 32)
	if err != nil {
		return srpProof{}, failure
	}
	defer clear(passwordKey)
	paddedGenerator := srpGenerator.FillBytes(make([]byte, 256))
	u := new(big.Int).SetBytes(sha(s.public.FillBytes(make([]byte, 256)), server.FillBytes(make([]byte, 256))))
	if u.Sign() == 0 {
		return srpProof{}, failure
	}
	k := new(big.Int).SetBytes(sha(srpGroup.Bytes(), paddedGenerator))
	x := new(big.Int).SetBytes(sha(salt, sha([]byte(":"), passwordKey)))
	base := new(big.Int).Exp(srpGenerator, x, srpGroup)
	base.Mul(base, k).Sub(server, base).Mod(base, srpGroup)
	if base.Sign() == 0 {
		return srpProof{}, failure
	}
	power := new(big.Int).Mul(u, x)
	power.Add(power, s.private)
	premaster := new(big.Int).Exp(base, power, srpGroup).Bytes()
	key := sha(premaster)
	nHash, gHash := sha(srpGroup.Bytes()), sha(paddedGenerator)
	for i := range nHash {
		nHash[i] ^= gHash[i]
	}
	clientProof := sha(nHash, sha([]byte(email)), salt, s.public.Bytes(), serverPublic, key)
	return srpProof{clientProof, sha(s.public.Bytes(), clientProof, key), key, premaster}, nil
}

func (p srpProof) decrypt(serverProof, encrypted []byte, target any) error {
	if !hmac.Equal(p.server, serverProof) || len(encrypted) == 0 || len(encrypted) > 256<<10 || len(encrypted)%aes.BlockSize != 0 {
		return &Error{Stage: "SMS_SERVER_PROOF"}
	}
	// Clients use either H(S) or the unpadded S for Apple's extra data labels.
	for _, secret := range [][]byte{p.key, p.premaster} {
		key := labelledHMAC(secret, "extra data key:")
		iv := labelledHMAC(secret, "extra data iv:")[:aes.BlockSize]
		block, _ := aes.NewCipher(key)
		plain := make([]byte, len(encrypted))
		cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, encrypted)
		padding := int(plain[len(plain)-1])
		if padding < 1 || padding > aes.BlockSize || !bytes.Equal(plain[len(plain)-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
			clear(plain)
			continue
		}
		payload := bytes.TrimSpace(plain[:len(plain)-padding])
		if bytes.HasPrefix(payload, []byte("<dict")) {
			payload = bytes.Join([][]byte{[]byte("<plist version=\"1.0\">"), payload, []byte("</plist>")}, nil)
		}
		_, err := plist.Unmarshal(payload, target)
		clear(payload)
		clear(plain)
		if err == nil {
			return nil
		}
	}
	return &Error{Stage: "SMS_SERVER_PROOF"}
}

func labelledHMAC(key []byte, label string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(label))
	return mac.Sum(nil)
}
