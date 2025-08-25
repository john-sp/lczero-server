package sprt

import (
	"testing"
)

func TestPentanomialSPRT(t *testing.T) {
	results := []int{39, 8843, 26675, 9240, 44}
	elo0 := 0.50
	elo1 := 2.50

	llr, err := PentanomialSPRT(results, elo0, elo1)
	if err != nil {
		t.Fatalf("PentanomialSPRT returned error: %v", err)
	}

	expected := 2.941675 // Pulled from running OpenBench's implementation
	tol := 0.5 * 1e-6

	diff := llr - expected
	if diff < 0 {
		diff = -diff
	}
	if diff > tol {
		t.Errorf("PentanomialSPRT = %v, want %v (diff %v > tol %v)", llr, expected, diff, tol)
	}
}
