//go:build cgo

package instrumentdomain

import "testing"

func TestSerializeUnserialize_RoundTrip(t *testing.T) {
	tests := []Domain{
		{Min: 0, Max: 0},
		{Min: -1, Max: 1},
		{Min: -123.456, Max: 789.012},
		{Min: -1e9, Max: 1e9},
	}

	for _, tc := range tests {
		serialized, err := tc.Serialize()
		if err != nil {
			t.Fatalf("Serialize(%+v) error = %v", tc, err)
		}

		got, err := unserialize(serialized)
		if err != nil {
			t.Fatalf("unserialize(%q) error = %v", serialized, err)
		}

		if got == nil {
			t.Fatalf("unserialize(%q) returned nil", serialized)
		}

		if got.Min != tc.Min {
			t.Errorf("Min mismatch: got %v, want %v", got.Min, tc.Min)
		}

		if got.Max != tc.Max {
			t.Errorf("Max mismatch: got %v, want %v", got.Max, tc.Max)
		}
	}
}
