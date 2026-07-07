// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

package idleendpoint

import (
	"context"
	"testing"
	"time"

	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/stretchr/testify/require"
)

func TestIdleEndpointImplementsEndpoint(_ *testing.T) {
	var _ endpoint.Endpoint = New()
}

func TestIdleEndpointCloseStopsRun(t *testing.T) {
	ep := New()
	done := make(chan error, 1)

	go func() {
		done <- ep.Run(context.Background())
	}()

	require.NoError(t, ep.Close())

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Run did not return after Close")
	}
}
