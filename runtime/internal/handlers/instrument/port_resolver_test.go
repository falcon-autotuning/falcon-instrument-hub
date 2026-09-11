package instrument

import (
	"strings"
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
)

func TestResolveConnectedPort(t *testing.T) {
	h := &Handler{
		PortConnections: connectedPortsFromTestWireMap(t),
	}

	tests := []struct {
		name         string
		deviceName   string
		ioTypeName   string
		role         string
		wantPortName ports.PortName
		wantChannel  int
	}{
		{
			name:         "meter slope setting",
			deviceName:   "O1",
			ioTypeName:   "slope",
			role:         "setting",
			wantPortName: "Mock.Meter1.analog.slope",
			wantChannel:  1,
		},
		{
			name:         "meter trigger level setting",
			deviceName:   "O1",
			ioTypeName:   "trigger_level",
			role:         "setting",
			wantPortName: "Mock.Meter1.analog.trigger_level",
			wantChannel:  1,
		},
		{
			name:         "meter voltage input",
			deviceName:   "O1",
			ioTypeName:   "voltage",
			role:         "input",
			wantPortName: "Mock.Meter1.analog.voltage",
			wantChannel:  1,
		},
		{
			name:         "source voltage output",
			deviceName:   "P1",
			ioTypeName:   "voltage",
			role:         "output",
			wantPortName: "Mock.Source1.analog.voltage",
			wantChannel:  4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := h.ResolveConnectedPort(tt.deviceName, tt.ioTypeName, tt.role)
			if err != nil {
				t.Fatalf("ResolveConnectedPort returned error: %v", err)
			}
			if got.PortName != tt.wantPortName {
				t.Fatalf("PortName = %q, want %q", got.PortName, tt.wantPortName)
			}
			if got.ChannelIndex != tt.wantChannel {
				t.Fatalf("ChannelIndex = %d, want %d", got.ChannelIndex, tt.wantChannel)
			}
			if got.DeviceName != tt.deviceName {
				t.Fatalf("DeviceName = %q, want %q", got.DeviceName, tt.deviceName)
			}
			if got.IoTypeName != tt.ioTypeName {
				t.Fatalf("IoTypeName = %q, want %q", got.IoTypeName, tt.ioTypeName)
			}
			if got.Role != tt.role {
				t.Fatalf("Role = %q, want %q", got.Role, tt.role)
			}
		})
	}
}

func connectedPortsFromTestWireMap(t *testing.T) []ports.ConnectedPort {
	t.Helper()

	apis := []ports.InstrumentAPI{
		{
			Instrument: ports.APIInstrument{
				Vendor:     "Mock",
				Identifier: "Meter1",
			},
			Protocol: ports.APIProtocol{
				Type: "MockMultimeter",
			},
			ChannelGroups: []ports.ChannelGroup{
				{
					Name: "analog",
					IoTypes: []ports.IoType{
						{Name: "voltage", Role: "input", Unit: "V"},
						{Name: "stream", Role: "input", Unit: "V"},
						{Name: "slope", Role: "setting"},
						{Name: "trigger_level", Role: "setting", Unit: "V"},
					},
				},
			},
		},
		{
			Instrument: ports.APIInstrument{
				Vendor:     "Mock",
				Identifier: "Source1",
			},
			Protocol: ports.APIProtocol{
				Type: "MockVoltageSource",
			},
			ChannelGroups: []ports.ChannelGroup{
				{
					Name: "analog",
					IoTypes: []ports.IoType{
						{Name: "voltage", Role: "output", Unit: "V"},
					},
				},
			},
		},
	}

	wireMap := map[string]string{
		"Meter1.analog.1":  "O1",
		"Source1.analog.4": "P1",
	}

	connected, err := ports.ConnectWireMap(wireMap, ports.BuildPortLibrary(apis))
	if err != nil {
		t.Fatalf("ConnectWireMap returned error: %v", err)
	}
	return connected
}

func TestResolveConnectedPortErrors(t *testing.T) {
	h := &Handler{
		PortConnections: []ports.ConnectedPort{
			{
				PortName:   "Mock.Meter1.analog.slope",
				DeviceName: "O1",
				IoTypeName: "slope",
				Role:       "setting",
			},
			{
				PortName:   "Mock.OtherMeter.analog.slope",
				DeviceName: "O1",
				IoTypeName: "slope",
				Role:       "setting",
			},
		},
	}

	if _, err := h.ResolveConnectedPort("O1", "voltage", "input"); err == nil {
		t.Fatal("expected no-match error")
	}

	_, err := h.ResolveConnectedPort("O1", "slope", "setting")
	if err == nil {
		t.Fatal("expected ambiguous-match error")
	}
	if !strings.Contains(err.Error(), "ambiguous connected port") {
		t.Fatalf("error = %q, want ambiguous connected port", err.Error())
	}
}
