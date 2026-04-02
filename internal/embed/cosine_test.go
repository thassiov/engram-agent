package embed

import (
	"math"
	"testing"
)

const floatTolerance = 1e-6

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < floatTolerance
}

func TestCosineSimilarity_Identical(t *testing.T) {
	v := []float32{1.0, 2.0, 3.0}
	result := CosineSimilarity(v, v)
	if !almostEqual(result, 1.0) {
		t.Errorf("identical vectors: want 1.0, got %f", result)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}
	result := CosineSimilarity(a, b)
	if !almostEqual(result, 0.0) {
		t.Errorf("orthogonal vectors: want 0.0, got %f", result)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{-1.0, 0.0}
	result := CosineSimilarity(a, b)
	if !almostEqual(result, -1.0) {
		t.Errorf("opposite vectors: want -1.0, got %f", result)
	}
}

func TestCosineSimilarity_DifferentLengths(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{1.0, 2.0}
	result := CosineSimilarity(a, b)
	if result != 0 {
		t.Errorf("different length vectors: want 0, got %f", result)
	}
}

func TestCosineSimilarity_EmptyVectors(t *testing.T) {
	result := CosineSimilarity([]float32{}, []float32{})
	if result != 0 {
		t.Errorf("empty vectors: want 0, got %f", result)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0.0, 0.0, 0.0}
	b := []float32{1.0, 2.0, 3.0}
	result := CosineSimilarity(a, b)
	if result != 0 {
		t.Errorf("zero vector: want 0, got %f", result)
	}
}
