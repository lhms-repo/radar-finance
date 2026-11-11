package auth

import (
	"strings"
	"testing"
)

// TestHashPasswordFormat checks that hashes use argon2id and a random salt (RNF-09).
func TestHashPasswordFormat(t *testing.T) {
	a, err := HashPassword("senha-ficticia-1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword("senha-ficticia-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(a, "$argon2id$v=19$") {
		t.Fatalf("unexpected hash format: %s", a)
	}
	if a == b {
		t.Fatal("two hashes of the same password must differ because of the salt")
	}
}

// TestVerifyPassword checks matching, mismatching and malformed hashes.
func TestVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correta-123")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		hash     string
		password string
		want     bool
		wantErr  bool
	}{
		{name: "correct password", hash: hash, password: "correta-123", want: true},
		{name: "wrong password", hash: hash, password: "errada-123", want: false},
		{name: "empty password", hash: hash, password: "", want: false},
		{name: "case matters", hash: hash, password: "CORRETA-123", want: false},
		{name: "malformed hash", hash: "$argon2id$v=19$xyz", password: "x", wantErr: true},
		{name: "other algorithm", hash: "$2a$10$abcdefghijklmnopqrstuv", password: "x", wantErr: true},
		{name: "bad base64", hash: "$argon2id$v=19$m=65536,t=3,p=2$!!!$!!!", password: "x", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := VerifyPassword(tt.hash, tt.password)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestValidatePassword checks the minimum password policy.
func TestValidatePassword(t *testing.T) {
	tests := []struct {
		password string
		wantErr  bool
	}{
		{password: "", wantErr: true},
		{password: "1234567", wantErr: true},
		{password: "12345678", wantErr: false},
		{password: strings.Repeat("a", 129), wantErr: true},
	}
	for _, tt := range tests {
		if err := ValidatePassword(tt.password); (err != nil) != tt.wantErr {
			t.Errorf("ValidatePassword(%q) err = %v, wantErr %v", tt.password, err, tt.wantErr)
		}
	}
}
