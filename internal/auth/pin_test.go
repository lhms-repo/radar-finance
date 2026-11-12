package auth

import (
	"strings"
	"testing"
)

// TestValidPIN accepts only 4 to 6 digits (RN-27).
func TestValidPIN(t *testing.T) {
	tests := []struct {
		pin  string
		want bool
	}{
		{"1234", true}, {"123456", true}, {"0000", true},
		{"123", false}, {"1234567", false}, {"12a4", false}, {" 1234", false}, {"", false}, {"١٢٣٤", false},
	}
	for _, tt := range tests {
		if got := ValidPIN(tt.pin); got != tt.want {
			t.Errorf("ValidPIN(%q) = %v, want %v", tt.pin, got, tt.want)
		}
	}
}

// TestHashPIN stores only a salted hash that checks the right PIN and refuses the others (RN-27).
func TestHashPIN(t *testing.T) {
	old := PINIterations
	PINIterations = 1_000
	defer func() { PINIterations = old }()

	a, err := HashPIN("4821")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := HashPIN("4821")
	if a == b {
		t.Fatal("two hashes of the same PIN are equal; the salt is missing")
	}
	if strings.Contains(a, "4821") {
		t.Fatal("the hash contains the PIN")
	}
	tests := []struct {
		name   string
		stored string
		pin    string
		want   bool
	}{
		{name: "right PIN", stored: a, pin: "4821", want: true},
		{name: "wrong PIN", stored: a, pin: "4822"},
		{name: "empty PIN", stored: a, pin: ""},
		{name: "no hash yet", stored: "", pin: "4821"},
		{name: "other format", stored: "bcrypt$x$y$z", pin: "4821"},
		{name: "broken rounds", stored: "pbkdf2-sha256$x$AAAA$AAAA", pin: "4821"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckPIN(tt.stored, tt.pin); got != tt.want {
				t.Fatalf("CheckPIN = %v, want %v", got, tt.want)
			}
		})
	}
	// A hash made with other rounds still checks: the count is stored with it.
	PINIterations = 2_000
	if !CheckPIN(a, "4821") {
		t.Fatal("changing the rounds broke an old hash")
	}
}
