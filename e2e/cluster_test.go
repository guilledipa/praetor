package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestNATSContainerStarts(t *testing.T) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "nats:2.10-alpine",
		ExposedPorts: []string{"4222/tcp", "8222/tcp"},
		WaitingFor:   wait.ForLog("Server is ready"),
		Cmd:          []string{"-js", "-m", "8222"}, // Enable JetStream and Monitoring
	}

	natsC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	defer func() {
		if err := natsC.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %s", err.Error())
		}
	}()

	// Get the mapped port
	port, err := natsC.MappedPort(ctx, "4222")
	require.NoError(t, err)

	t.Logf("NATS started on port: %s", port.Port())
	
	// Wait a tiny bit to ensure it stays up
	time.Sleep(1 * time.Second)
	
	require.True(t, natsC.IsRunning())
}
