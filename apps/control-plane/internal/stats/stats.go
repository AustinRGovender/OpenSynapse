// Package stats has the small amount of statistics run comparison needs.
package stats

import "math"

// MeanVar returns the sample mean and (unbiased) variance.
func MeanVar(xs []float64) (mean, variance float64) {
	n := float64(len(xs))
	if n == 0 {
		return 0, 0
	}
	for _, x := range xs {
		mean += x
	}
	mean /= n
	if n < 2 {
		return mean, 0
	}
	for _, x := range xs {
		d := x - mean
		variance += d * d
	}
	return mean, variance / (n - 1)
}

// WelchTTest returns the two-tailed p-value for the difference in means of
// two samples with unequal variances. It returns 1 when there is not enough
// data to say anything.
func WelchTTest(a, b []float64) float64 {
	if len(a) < 2 || len(b) < 2 {
		return 1
	}
	ma, va := MeanVar(a)
	mb, vb := MeanVar(b)
	na, nb := float64(len(a)), float64(len(b))
	se2 := va/na + vb/nb
	if se2 == 0 {
		if ma == mb {
			return 1
		}
		return 0 // identical constant samples that differ: certainly different
	}
	t := (ma - mb) / math.Sqrt(se2)
	df := se2 * se2 / ((va*va)/(na*na*(na-1)) + (vb*vb)/(nb*nb*(nb-1)))
	return StudentTwoTailedP(t, df)
}

// StudentTwoTailedP is P(|T| >= |t|) for Student's t with df degrees of freedom.
func StudentTwoTailedP(t, df float64) float64 {
	x := df / (df + t*t)
	return RegIncBeta(df/2, 0.5, x)
}

// RegIncBeta is the regularized incomplete beta function I_x(a, b).
func RegIncBeta(a, b, x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	lbeta, _ := math.Lgamma(a + b)
	la, _ := math.Lgamma(a)
	lb, _ := math.Lgamma(b)
	front := math.Exp(lbeta - la - lb + a*math.Log(x) + b*math.Log(1-x))
	if x < (a+1)/(a+b+2) {
		return front * betaCF(a, b, x) / a
	}
	return 1 - front*betaCF(b, a, 1-x)/b
}

// betaCF evaluates the continued fraction for the incomplete beta function
// (modified Lentz's method).
func betaCF(a, b, x float64) float64 {
	const (
		maxIter = 300
		eps     = 3e-14
		fpmin   = 1e-300
	)
	qab, qap, qam := a+b, a+1, a-1
	c, d := 1.0, 1-qab*x/qap
	if math.Abs(d) < fpmin {
		d = fpmin
	}
	d = 1 / d
	h := d
	for m := 1; m <= maxIter; m++ {
		fm := float64(m)
		m2 := 2 * fm
		aa := fm * (b - fm) * x / ((qam + m2) * (a + m2))
		d = 1 + aa*d
		if math.Abs(d) < fpmin {
			d = fpmin
		}
		c = 1 + aa/c
		if math.Abs(c) < fpmin {
			c = fpmin
		}
		d = 1 / d
		h *= d * c
		aa = -(a + fm) * (qab + fm) * x / ((a + m2) * (qap + m2))
		d = 1 + aa*d
		if math.Abs(d) < fpmin {
			d = fpmin
		}
		c = 1 + aa/c
		if math.Abs(c) < fpmin {
			c = fpmin
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < eps {
			break
		}
	}
	return h
}
