package store

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestGenerateRandomKey(t *testing.T) {
	k1, err := GenerateRandomKey()
	if err != nil {
		t.Fatalf("GenerateRandomKey: %v", err)
	}
	k2, err := GenerateRandomKey()
	if err != nil {
		t.Fatalf("GenerateRandomKey: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(k1)
	if err != nil {
		t.Fatalf("key is not valid base64: %v", err)
	}
	if len(raw) != 32 {
		t.Fatalf("key length = %d, want 32", len(raw))
	}
	if k1 == k2 {
		t.Fatal("two generated keys should differ")
	}
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	master, err := GenerateRandomKey()
	if err != nil {
		t.Fatalf("GenerateRandomKey: %v", err)
	}
	cases := []string{
		"sk-abc123",
		"",
		strings.Repeat("x", 4096),
		"中文密钥内容🔐",
	}
	for _, plain := range cases {
		cipherB64, err := Encrypt(plain, master)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", plain, err)
		}
		got, err := Decrypt(cipherB64, master)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if got != plain {
			t.Fatalf("roundtrip mismatch: got %q, want %q", got, plain)
		}
		if plain != "" && strings.Contains(cipherB64, plain) {
			t.Fatalf("ciphertext leaks plaintext: %q", cipherB64)
		}
	}
}

func TestDecryptWrongKey(t *testing.T) {
	master1, _ := GenerateRandomKey()
	master2, _ := GenerateRandomKey()
	cipherB64, err := Encrypt("sk-secret", master1)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := Decrypt(cipherB64, master2); err == nil {
		t.Fatal("decrypt with wrong key should fail")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	master, _ := GenerateRandomKey()
	cipherB64, err := Encrypt("sk-secret", master)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(cipherB64)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	raw[len(raw)-1] ^= 0xFF // 翻转最后一个密文字节
	tampered := base64.StdEncoding.EncodeToString(raw)
	if _, err := Decrypt(tampered, master); err == nil {
		t.Fatal("tampered ciphertext should fail GCM authentication")
	}
}

func TestDecryptInvalidInputs(t *testing.T) {
	master, _ := GenerateRandomKey()
	if _, err := Decrypt("not-base64!!!", master); err == nil {
		t.Fatal("invalid base64 should fail")
	}
	if _, err := Decrypt(base64.StdEncoding.EncodeToString([]byte("short")), master); err == nil {
		t.Fatal("too-short ciphertext should fail")
	}
	if _, err := Decrypt("anything", "invalid-master-key"); err == nil {
		t.Fatal("invalid master key should fail")
	}
	shortKey := base64.StdEncoding.EncodeToString([]byte("16-bytes-key!!"))
	if _, err := Decrypt("anything", shortKey); err == nil {
		t.Fatal("non-32-byte master key should fail")
	}
}
