package model

import (
	"errors"
	"math"
)

var errNotPositiveDefinite = errors.New("matrix is not positive definite")

// solveSPD solves A x = b for a symmetric positive-definite A using Cholesky
// decomposition. A is modified in place. The matrices here are factors x factors
// (32x32 by default), so this runs in microseconds per request.
func solveSPD(a [][]float64, b []float64) ([]float64, error) {
	n := len(b)
	l := make([][]float64, n)
	for i := range l {
		l[i] = make([]float64, n)
	}

	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {
			sum := a[i][j]
			for k := 0; k < j; k++ {
				sum -= l[i][k] * l[j][k]
			}
			if i == j {
				if sum <= 0 || math.IsNaN(sum) {
					return nil, errNotPositiveDefinite
				}
				l[i][j] = math.Sqrt(sum)
			} else {
				l[i][j] = sum / l[j][j]
			}
		}
	}

	// Forward substitution: L y = b.
	y := make([]float64, n)
	for i := 0; i < n; i++ {
		sum := b[i]
		for k := 0; k < i; k++ {
			sum -= l[i][k] * y[k]
		}
		y[i] = sum / l[i][i]
	}

	// Back substitution: L^T x = y.
	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		sum := y[i]
		for k := i + 1; k < n; k++ {
			sum -= l[k][i] * x[k]
		}
		x[i] = sum / l[i][i]
	}
	return x, nil
}

func dot(a, b []float64) float64 {
	var sum float64
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

func norm(a []float64) float64 {
	return math.Sqrt(dot(a, a))
}

func cosine(a, b []float64) float64 {
	d := norm(a) * norm(b)
	if d == 0 {
		return 0
	}
	return dot(a, b) / d
}
