package datastore

import (
	"bufio"
	"bytes"
	"testing"
)

func TestEntry_Encode(t *testing.T) {
	e := entry{"key", "value"}
	encoded := e.Encode()

	var decoded entry
	decoded.Decode(encoded)

	if decoded.key != "key" {
		t.Error("incorrect key")
	}
	if decoded.value != "value" {
		t.Error("incorrect value")
	}
}

func TestReadValue(t *testing.T) {
	a := entry{"key", "test-value"}
	originalBytes := a.Encode()

	var b entry
	b.Decode(originalBytes)
	t.Log("encode/decode", a, b)
	if a != b {
		t.Error("Encode/Decode mismatch")
	}

	var c entry
	n, err := c.DecodeFromReader(bufio.NewReader(bytes.NewReader(originalBytes)))
	if err != nil {
		t.Fatal(err)
	}
	t.Log("encode/decodeFromReader", a, c)
	if a != c {
		t.Error("Encode/DecodeFromReader mismatch")
	}
	if n != len(originalBytes) {
		t.Errorf("DecodeFromReader() read %d bytes, expected %d", n, len(originalBytes))
	}
}
