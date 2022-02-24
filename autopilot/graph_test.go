package autopilot_test

import (
	"testing"

	"github.com/ltcsuite/lnd/autopilot"
	"github.com/ltcsuite/ltcd/ltcutil"
)

// TestMedian tests the Median method.
func TestMedian(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		values []ltcutil.Amount
		median ltcutil.Amount
	}{
		{
			values: []ltcutil.Amount{},
			median: 0,
		},
		{
			values: []ltcutil.Amount{10},
			median: 10,
		},
		{
			values: []ltcutil.Amount{10, 20},
			median: 15,
		},
		{
			values: []ltcutil.Amount{10, 20, 30},
			median: 20,
		},
		{
			values: []ltcutil.Amount{30, 10, 20},
			median: 20,
		},
		{
			values: []ltcutil.Amount{10, 10, 10, 10, 5000000},
			median: 10,
		},
	}

	for _, test := range testCases {
		res := autopilot.Median(test.values)
		if res != test.median {
			t.Fatalf("expected median %v, got %v", test.median, res)
		}
	}
}
