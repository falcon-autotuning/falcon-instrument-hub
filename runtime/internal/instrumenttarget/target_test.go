//go:build cgo

package instrumenttarget

import (
	"strings"
	"testing"
)

func TestSerializeDeserialize_RoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		target Target
	}{
		{
			name: "basic",
			target: Target{
				Instrument: "DAQ1",
				Group:      "analog",
				Channel:    0,
			},
		},
		{
			name: "positive channel",
			target: Target{
				Instrument: "PXI-6738",
				Group:      "ao",
				Channel:    13,
			},
		},
		{
			name: "negative channel",
			target: Target{
				Instrument: "DeviceA",
				Group:      "custom",
				Channel:    -7,
			},
		},
		{
			name: "long valid strings",
			target: Target{
				Instrument: strings.Repeat("I", maxStringBytes),
				Group:      strings.Repeat("G", maxStringBytes),
				Channel:    12345,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			serialized, err := tc.target.Serialize()
			if err != nil {
				t.Fatalf("Serialize() returned error: %v", err)
			}

			if serialized == "" {
				t.Fatal("Serialize() returned empty string")
			}

			got, err := deserialize(serialized)
			if err != nil {
				t.Fatalf("deserialize() returned error: %v", err)
			}

			if got == nil {
				t.Fatal("deserialize() returned nil")
			}

			if got.Instrument != tc.target.Instrument {
				t.Errorf(
					"Instrument mismatch: got %q want %q",
					got.Instrument,
					tc.target.Instrument,
				)
			}

			if got.Group != tc.target.Group {
				t.Errorf(
					"Group mismatch: got %q want %q",
					got.Group,
					tc.target.Group,
				)
			}

			if got.Channel != tc.target.Channel {
				t.Errorf(
					"Channel mismatch: got %d want %d",
					got.Channel,
					tc.target.Channel,
				)
			}
		})
	}
}

func TestSerialize_ValidationErrors(t *testing.T) {
	tests := []struct {
		name   string
		target Target
	}{
		{
			name: "empty instrument",
			target: Target{
				Instrument: "",
				Group:      "analog",
				Channel:    0,
			},
		},
		{
			name: "empty group",
			target: Target{
				Instrument: "DAQ1",
				Group:      "",
				Channel:    0,
			},
		},
		{
			name: "instrument contains delimiter",
			target: Target{
				Instrument: "DAQ|1",
				Group:      "analog",
				Channel:    0,
			},
		},
		{
			name: "group contains delimiter",
			target: Target{
				Instrument: "DAQ1",
				Group:      "an|alog",
				Channel:    0,
			},
		},
		{
			name: "instrument too long",
			target: Target{
				Instrument: strings.Repeat("A", maxStringBytes+1),
				Group:      "analog",
				Channel:    0,
			},
		},
		{
			name: "group too long",
			target: Target{
				Instrument: "DAQ1",
				Group:      strings.Repeat("A", maxStringBytes+1),
				Channel:    0,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.target.Serialize(); err == nil {
				t.Fatal("expected Serialize() to fail")
			}
		})
	}
}
