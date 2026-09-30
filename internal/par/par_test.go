package par

import "testing"

func TestWaitRepanicsInCaller(t *testing.T) {
	defer func() {
		if r := recover(); r != "boom" {
			t.Fatalf("recovered %v; want boom", r)
		}
	}()
	var g Group
	for i := range 8 {
		g.Go(func() {
			if i == 3 {
				panic("boom")
			}
		})
	}
	g.Wait()
	t.Fatal("Wait returned after a goroutine panicked")
}

func TestWaitWithoutPanic(t *testing.T) {
	var g Group
	n := make([]int, 8)
	for i := range n {
		g.Go(func() { n[i] = i })
	}
	g.Wait()
	for i, v := range n {
		if v != i {
			t.Fatalf("n[%d] = %d", i, v)
		}
	}
}
