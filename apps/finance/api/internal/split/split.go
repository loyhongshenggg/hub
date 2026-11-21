// Package split divides a transaction between you (the payer) and others,
// in whole cents. Like Splitwise, each person has a share they owe; your
// share is your spending, the others' are money owed to you (a receivable,
// not your expense).
package split

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

// Methods.
const (
	Equal  = "equal"  // everyone pays the same
	Shares = "shares" // by weights (2:1, or percentages)
	Exact  = "exact"  // fixed amounts for the others; you pay the rest
)

// Part is one other person in a split.
type Part struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight,omitempty"` // equal: ignored; shares: weight
	Amount int64   `json:"amount,omitempty"` // exact: cents
}

// Result is the computed split. Amounts carry the transaction's sign.
type Result struct {
	Mine   int64            `json:"mine"`
	Others map[string]int64 `json:"others"` // by name
}

// Compute splits total (cents, any sign) by method. myWeight is your weight
// for shares; for equal you count as one person. Remainder cents go first
// to the largest fractional parts, ties to you, so shares sum exactly.
func Compute(total int64, method string, myWeight float64, parts []Part) (Result, error) {
	if len(parts) == 0 {
		return Result{}, errors.New("add at least one other person")
	}
	seen := map[string]bool{}
	for i := range parts {
		parts[i].Name = strings.TrimSpace(parts[i].Name)
		key := strings.ToLower(parts[i].Name)
		if parts[i].Name == "" {
			return Result{}, errors.New("every person needs a name")
		}
		if seen[key] || key == "me" || key == "you" {
			return Result{}, fmt.Errorf("%q is listed twice", parts[i].Name)
		}
		seen[key] = true
	}
	sign := int64(1)
	if total < 0 {
		sign, total = -1, -total
	}
	res := Result{Others: map[string]int64{}}
	switch method {
	case Exact:
		var sum int64
		for _, p := range parts {
			if p.Amount < 0 {
				return Result{}, errors.New("amounts can't be negative")
			}
			res.Others[p.Name] = p.Amount * sign
			sum += p.Amount
		}
		if sum > total {
			return Result{}, fmt.Errorf("the others' amounts (%s) are more than the transaction (%s)", cents(sum), cents(total))
		}
		res.Mine = (total - sum) * sign
		return res, nil
	case Equal, Shares:
		weights := make([]float64, len(parts)+1)
		weights[0] = 1 // you
		if method == Shares {
			weights[0] = myWeight
		}
		for i, p := range parts {
			w := 1.0
			if method == Shares {
				w = p.Weight
			}
			weights[i+1] = w
		}
		alloc, err := largestRemainder(total, weights)
		if err != nil {
			return Result{}, err
		}
		res.Mine = alloc[0] * sign
		for i, p := range parts {
			res.Others[p.Name] = alloc[i+1] * sign
		}
		return res, nil
	}
	return Result{}, fmt.Errorf("unknown split method %q", method)
}

// largestRemainder divides total (≥ 0) by weights, in whole units.
func largestRemainder(total int64, weights []float64) ([]int64, error) {
	var sum float64
	for _, w := range weights {
		if w < 0 || math.IsNaN(w) || math.IsInf(w, 0) {
			return nil, errors.New("shares can't be negative")
		}
		sum += w
	}
	if sum <= 0 {
		return nil, errors.New("shares add up to zero")
	}
	out := make([]int64, len(weights))
	type frac struct {
		i int
		f float64
	}
	fr := make([]frac, len(weights))
	var given int64
	for i, w := range weights {
		exact := float64(total) * w / sum
		out[i] = int64(math.Floor(exact))
		given += out[i]
		fr[i] = frac{i, exact - float64(out[i])}
	}
	sort.SliceStable(fr, func(a, b int) bool { return fr[a].f > fr[b].f+1e-9 })
	for k := 0; given < total; k++ {
		out[fr[k%len(fr)].i]++
		given++
	}
	return out, nil
}

func cents(c int64) string { return fmt.Sprintf("S$%d.%02d", c/100, c%100) }
