//go:build cgo && falcon_core

package callstack

import "testing"

func TestDescriptorSerialize(t *testing.T) {
	got, err := (Descriptor{
		Instrument: "Meter1",
		Group:      "analog",
		Channel:    2,
		Command:    "GET_VOLTAGE",
	}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	if got != "Meter1|analog|2|GET_VOLTAGE" {
		t.Fatalf("serialized CallStack = %q", got)
	}
}

func TestDescriptorSerializeRejectsInvalidFields(t *testing.T) {
	tests := []Descriptor{
		{Group: "analog", Channel: 1, Command: "GET_VOLTAGE"},
		{Instrument: "Meter1", Channel: 1, Command: "GET_VOLTAGE"},
		{Instrument: "Meter|1", Group: "analog", Channel: 1, Command: "GET_VOLTAGE"},
		{Instrument: "Meter1", Group: "analog", Channel: 1, Command: ""},
	}

	for _, descriptor := range tests {
		if _, err := descriptor.Serialize(); err == nil {
			t.Fatalf("Serialize(%+v) succeeded", descriptor)
		}
	}
}
