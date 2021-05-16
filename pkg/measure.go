package pkg

import (
	"sort"
)

type Measure []int64

func (a Measure) Len() int           { return len(a) }
func (a Measure) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a Measure) Less(i, j int) bool { return a[i] < a[j] }

func (a Measure) Stats() (int64, int64, int64, int64) {
	if len(a) == 0 {
		panic("Measure.Stats() called on empty array")
	}
	sort.Sort(a)
	max := a[len(a)-1]
	p99 := a[(len(a)-1)*99/100]
	p95 := a[(len(a)-1)*95/100]
	avg := int64(0)
	for i := 0; i < len(a); i++ {
		avg = avg + a[i]
	}
	avg = avg / int64(len(a))
	return max, p99, p95, avg
}
