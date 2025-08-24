// Package sprt provides statistical functions for SPRT testing.
package sprt

import (
	"errors"
	"math"

	"gonum.org/v1/gonum/stat/distuv"
)

// From https://github.com/AndyGrant/OpenBench/blob/master/OpenBench/stats.py converted to Go.
// Only two functions should be used externally from this Module.
// 1. llr = PentanomialSPRT([ll, ld, dd, dw, ww], elo0, elo1)
// 2. lower, elo, upper = Elo((L, D, W) or (LL, LD, DD/WL, DW, WW)) (Not implemented)

// PentanomialSPRT implements the pentanomial SPRT as described in
// https://hardy.uhasselt.be/Fishtest/normalized_elo_practical.pdf
func PentanomialSPRT(results []int, elo0, elo1 float64) (float64, error) {
	if len(results) != 5 {
		return 0, errors.New("results must have length 5")
	}

	// Avoid division by zero
	r := make([]float64, 5)
	for i, x := range results {
		val := float64(x)
		if val < 1e-3 {
			val = 1e-3
		}
		r[i] = val
	}

	neloDividedByNt := 800.0 / math.Log(10)
	nt0 := elo0 / neloDividedByNt
	nt1 := elo1 / neloDividedByNt
	t0 := nt0 * math.Sqrt(2)
	t1 := nt1 * math.Sqrt(2)

	N := 0.0
	for _, v := range r {
		N += v
	}

	pdf := make([][2]float64, 5)
	for i := 0; i < 5; i++ {
		pdf[i][0] = float64(i) / 4.0
		pdf[i][1] = r[i] / N
	}

	pdf0, err := MLE_tvalue(pdf, 0.5, t0)
	if err != nil {
		return 0, err
	}
	pdf1, err := MLE_tvalue(pdf, 0.5, t1)
	if err != nil {
		return 0, err
	}

	mlePDF := make([][2]float64, len(pdf))
	for i := range pdf {
		mlePDF[i][0] = math.Log(pdf1[i][1]) - math.Log(pdf0[i][1])
		mlePDF[i][1] = pdf[i][1]
	}

	s, _ := stats(mlePDF)
	return N * s, nil
}

// MLE_tvalue computes the maximum likelihood estimate for a given t-value.
func MLE_tvalue(pdfhat [][2]float64, ref, s float64) ([][2]float64, error) {
	N := len(pdfhat)
	pdfMLE := uniform(pdfhat)
	for iter := 0; iter < 10; iter++ {
		prev := make([][2]float64, N)
		copy(prev, pdfMLE)
		mu, var_ := stats(pdfMLE)
		sigma := math.Sqrt(var_)
		pdf1 := make([][2]float64, N)
		for i, v := range pdfhat {
			ai := v[0]
			pdf1[i][0] = ai - ref - s*sigma*(1+math.Pow((mu-ai)/sigma, 2))/2
			pdf1[i][1] = v[1]
		}
		x, err := secular(pdf1)
		if err != nil {
			return nil, err
		}
		for i := range pdfMLE {
			pdfMLE[i][0] = pdfhat[i][0]
			pdfMLE[i][1] = pdfhat[i][1] / (1 + x*pdf1[i][0])
		}
		maxDiff := 0.0
		for i := range pdfMLE {
			d := math.Abs(prev[i][1] - pdfMLE[i][1])
			if d > maxDiff {
				maxDiff = d
			}
		}
		if maxDiff < 1e-9 {
			break
		}
	}
	return pdfMLE, nil
}

// stats computes the mean and variance of a pdf.
func stats(pdf [][2]float64) (mean, variance float64) {
	epsilon := 1e-6
	n := 0.0
	for _, v := range pdf {
		if v[1] < -epsilon || v[1] > 1+epsilon {
			panic("probability out of bounds")
		}
		n += v[1]
	}
	if math.Abs(n-1) > epsilon {
		panic("probabilities do not sum to 1")
	}
	s := 0.0
	for _, v := range pdf {
		s += v[1] * v[0]
	}
	var_ := 0.0
	for _, v := range pdf {
		var_ += v[1] * math.Pow(v[0]-s, 2)
	}
	return s, var_
}

// uniform returns a uniform pdf with the same support as pdf.
func uniform(pdf [][2]float64) [][2]float64 {
	n := float64(len(pdf))
	out := make([][2]float64, len(pdf))
	for i, v := range pdf {
		out[i][0] = v[0]
		out[i][1] = 1.0 / n
	}
	return out
}

// secular solves sum_i pi*ai/(1+x*ai)=0 for x using Brent's method.
func secular(pdf [][2]float64) (float64, error) {
	epsilon := 1e-9
	v := pdf[0][0]
	w := pdf[len(pdf)-1][0]
	for _, v2 := range pdf {
		if v2[0] < v {
			v = v2[0]
		}
		if v2[0] > w {
			w = v2[0]
		}
	}
	if v*w >= 0 {
		return 0, errors.New("secular: v*w >= 0")
	}
	l := -1.0 / w
	u := -1.0 / v

	f := func(x float64) float64 {
		sum := 0.0
		for _, v := range pdf {
			ai, pi := v[0], v[1]
			sum += pi * ai / (1 + x*ai)
		}
		return sum
	}

	// Brent's method
	const maxIter = 100
	a, b := l+epsilon, u-epsilon
	fa, fb := f(a), f(b)
	if fa*fb > 0 {
		return 0, errors.New("secular: root not bracketed")
	}
	for i := 0; i < maxIter; i++ {
		c := (a + b) / 2
		fc := f(c)
		if math.Abs(fc) < 1e-12 || (b-a)/2 < 1e-12 {
			return c, nil
		}
		if fa*fc < 0 {
			b, fb = c, fc
		} else {
			a, fa = c, fc
		}
	}
	return 0, errors.New("secular: did not converge")
}

// Elo computes the logistic Elo and confidence interval for a set of results.
// Returns (elo_min, elo, elo_max).
func Elo(results []int) (float64, float64, float64) {
	N := 0
	for _, v := range results {
		N += v
	}
	if N == 0 || N == 1 {
		return 0.0, 0.0, 0.0
	}
	div := float64(len(results) - 1)
	mu := 0.0
	for f, count := range results {
		mu += (float64(f) / div) * float64(count)
	}
	mu /= float64(N)
	variance := 0.0
	for f, count := range results {
		diff := (float64(f)/div - mu)
		variance += diff * diff * float64(count)
	}
	variance /= float64(N)
	df := float64(N - 1)
	t := distuv.StudentsT{Mu: 0, Sigma: 1, Nu: df}
	mu_min := mu + t.Quantile(0.025)*math.Sqrt(variance)/math.Sqrt(float64(N))
	mu_max := mu + t.Quantile(0.975)*math.Sqrt(variance)/math.Sqrt(float64(N))
	return logisticElo(mu_min), logisticElo(mu), logisticElo(mu_max)
}

// logisticElo converts a probability to logistic Elo.
func logisticElo(x float64) float64 {
	x = math.Min(math.Max(x, 1e-3), 1-1e-3)
	return -400 * math.Log10(1/x-1)
}
