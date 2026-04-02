package state

import (
	"math"
	"testing"
)

func TestEncodeDecodeVector_Roundtrip(t *testing.T) {
	original := []float32{1.5, -2.75, 0.0, 3.14159, math.MaxFloat32}
	encoded := encodeVector(original)
	decoded := decodeVector(encoded, len(original))

	if len(decoded) != len(original) {
		t.Fatalf("decoded length %d != original length %d", len(decoded), len(original))
	}
	for i := range original {
		if decoded[i] != original[i] {
			t.Errorf("mismatch at index %d: want %f, got %f", i, original[i], decoded[i])
		}
	}
}

func TestEncodeDecodeVector_Empty(t *testing.T) {
	var empty []float32
	encoded := encodeVector(empty)
	if len(encoded) != 0 {
		t.Errorf("expected empty byte slice for empty vector, got %d bytes", len(encoded))
	}
	decoded := decodeVector(encoded, 0)
	if len(decoded) != 0 {
		t.Errorf("expected empty float slice for empty bytes, got %d elements", len(decoded))
	}
}
