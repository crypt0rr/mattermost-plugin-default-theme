package main

import (
	"testing"

	"github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/require"
)

func TestMainEntrypoint(t *testing.T) {
	originalClientMain := clientMain
	t.Cleanup(func() {
		clientMain = originalClientMain
	})

	var implementation any
	var options []func(*plugin.ServeConfig) error
	clientMain = func(candidate any, candidateOptions ...func(*plugin.ServeConfig) error) {
		implementation = candidate
		options = candidateOptions
	}

	main()

	require.IsType(t, &Plugin{}, implementation)
	require.Empty(t, options)
}
