package split

import "testing"

func TestEqualRemainderGoesToPayer(t *testing.T) {
	r, err := Compute(10000, Equal, 0, []Part{{Name: "Alice"}, {Name: "Bob"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Mine != 3334 || r.Others["Alice"] != 3333 || r.Others["Bob"] != 3333 {
		t.Errorf("%+v", r)
	}
}

func TestSharesAndRefundSign(t *testing.T) {
	// 2:1 in my favour on a S$90 dinner.
	r, err := Compute(9000, Shares, 1, []Part{{Name: "Alice", Weight: 2}})
	if err != nil || r.Mine != 3000 || r.Others["Alice"] != 6000 {
		t.Errorf("%+v %v", r, err)
	}
	// Percentages are weights too; a refund keeps its negative sign.
	r, err = Compute(-1000, Shares, 30, []Part{{Name: "Alice", Weight: 70}})
	if err != nil || r.Mine != -300 || r.Others["Alice"] != -700 {
		t.Errorf("refund %+v %v", r, err)
	}
}

func TestExact(t *testing.T) {
	r, err := Compute(15000, Exact, 0, []Part{{Name: "Mum", Amount: 4000}, {Name: "Others", Amount: 6000}})
	if err != nil || r.Mine != 5000 {
		t.Errorf("%+v %v", r, err)
	}
	if _, err := Compute(15000, Exact, 0, []Part{{Name: "Mum", Amount: 20000}}); err == nil {
		t.Error("over-allocation accepted")
	}
}

func TestValidation(t *testing.T) {
	for _, parts := range [][]Part{nil, {{Name: ""}}, {{Name: "A"}, {Name: "a"}}, {{Name: "me"}}} {
		if _, err := Compute(100, Equal, 0, parts); err == nil {
			t.Errorf("accepted %+v", parts)
		}
	}
	if _, err := Compute(100, Shares, 0, []Part{{Name: "A", Weight: 0}}); err == nil {
		t.Error("zero weights accepted")
	}
	if _, err := Compute(100, "half", 0, []Part{{Name: "A"}}); err == nil {
		t.Error("unknown method accepted")
	}
}

func TestSumsExactly(t *testing.T) {
	for _, total := range []int64{1, 7, 99, 100, 101, 12345, 99999} {
		r, err := Compute(total, Shares, 1.5, []Part{{Name: "A", Weight: 2.25}, {Name: "B", Weight: 0.333}, {Name: "C", Weight: 7}})
		if err != nil {
			t.Fatal(err)
		}
		sum := r.Mine
		for _, v := range r.Others {
			sum += v
		}
		if sum != total {
			t.Errorf("total %d: shares sum to %d", total, sum)
		}
	}
}
