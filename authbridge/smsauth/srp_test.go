package smsauth

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"testing"
)

// Fixed SRP exchanges generated with the pinned SideInstaller reference,
// with client exponent 42, server exponent 101 and a synthetic account.
func TestSRPReferenceVectors(t *testing.T) {
	const email = "fixture@example.invalid"
	const password = "fixture-password"
	salt, _ := hex.DecodeString("0102030405060708090a0b0c0d0e0f10")
	vectors := []struct{ protocol, public, client, server string }{
		{"s2k",
			"2e58dab0bd5c0c6519df91e5d6f0979c1ab807d910f3fa1a0c40464b5ae3993a7bf4e121fa796f52207d5efc7f5f63db9d205e9058d7e3e9cc2df59eefffc9f9c47a112e9590d3e469c34418decc5df582ce0135959c59fc06a84bcfe722798aaac9af840413b19c4920b760c9b8d76e4b3c45368a5ac933ca886a7c45501528965e107528ed122b5481451b655782aa26923bdd519380a48be4691d77ac20dfc36c1336e3d234e18afadb2b41d84798ddd4795156ce061d31233e39b61d7769c66263fbe0b8cb75e86f1602e86c297406d11ec60ec12cd328b28ff4d0dc212420388c71ac966a07278c05ecf200e6b54b4bfca7c388a6c6fa3ee6b928595075",
			"b67640c6f557959cc8f90881e5a1b529f7039085c518ec0da4459c474145bb04",
			"5b46eabeafc18add2e16fc22627591ae913c62aacda5df598f019aca40d74180"},
		{"s2k_fo",
			"289b3180bf0859cde476d93cdbf000687df8dbd568984564da59a7e792f14a81b27beb4627d2b3801c8279b90c33368a2bdbc684195614bf67dd6dbe65eea4582608be541d36bb45539a392ff4040a3b4451d8d9567710fdfec5095586804b687f82b67dabb2c40537f6f54a35e41e1882869149fd44607339699e80262b76ae05c450bf86d7ee4e901a7c124a5e3ed720f4fff833f2d3dba0baacbc34815e47c962394555bbd626c8339c55ff67711f9a7037125f4ce7f5e0404af4222274f3c2738884fe9048c92c973a9e74d395b8e48928952ea1de426270c0d7bf4ba802c9d480d5d6abafd7a98eb85cfbbd011c3c7990142dc81acd3fc6e591a09ec331",
			"d0ef9c69d9a899473db0f8f55ecdb5d9019c1bec2ae138412cc4d7af6891545c",
			"0a9a5844162e378ab50b1d8be97f869b9928787289abd7a5fe6c00b6d7917e87"},
	}
	for _, v := range vectors {
		t.Run(v.protocol, func(t *testing.T) {
			public, _ := hex.DecodeString(v.public)
			proof, err := srpFromSecret([]byte{42}).proof(email, password, v.protocol, salt, public, 1000)
			if err != nil || hex.EncodeToString(proof.client) != v.client || hex.EncodeToString(proof.server) != v.server {
				t.Fatal("SRP proof differs from the reference")
			}
			payload := []byte("<dict><key>adsid</key><string>12345</string></dict>")
			padding := aes.BlockSize - len(payload)%aes.BlockSize
			for i := 0; i < padding; i++ {
				payload = append(payload, byte(padding))
			}
			block, _ := aes.NewCipher(labelledHMAC(proof.key, "extra data key:"))
			cipher.NewCBCEncrypter(block, labelledHMAC(proof.key, "extra data iv:")[:aes.BlockSize]).CryptBlocks(payload, payload)
			var values map[string]any
			if err = proof.decrypt(make([]byte, 32), payload, &values); !errorIsStage(err, "SMS_SERVER_PROOF") || values != nil {
				t.Fatal("unauthenticated server data was accepted")
			}
			if err = proof.decrypt(proof.server, payload, &values); err != nil || values["adsid"] != "12345" {
				t.Fatal("authenticated extra data was not decoded")
			}
		})
	}
}
