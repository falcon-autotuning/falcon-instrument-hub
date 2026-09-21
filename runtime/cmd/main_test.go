package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartFlagsMatchCurrentInstrumentRuntime(t *testing.T) {
	assert.Nil(t, startCmd.Flags().Lookup("packages"), "Python template flag must remain removed")
	for _, name := range []string{"inst-config", "inst-plugins", "instrument-apis", "user-measurement-luas"} {
		assert.NotNil(t, startCmd.Flags().Lookup(name))
	}
	diagnostics := startCmd.Flags().Lookup("log-diagnostics")
	require.NotNil(t, diagnostics)
	assert.Equal(t, "false", diagnostics.DefValue)
	assert.Contains(t, startCmd.Short, "Instrument Hub")
}
