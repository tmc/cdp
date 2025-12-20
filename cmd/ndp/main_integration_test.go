//go:build integration
// +build integration

package main

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func TestNodeFlagsCmd(t *testing.T) {
	// Build the ndp binary
	cmd := exec.Command("go", "build", "-o", "/tmp/ndp_test", ".")
	err := cmd.Run()
	if err != nil {
		t.Fatalf("Failed to build ndp binary: %v", err)
	}

	// Run the node-flags command
	cmd = exec.Command("/tmp/ndp_test", "node", "node-flags")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run node-flags command: %v\nOutput:\n%s", err, string(output))
	}

	// Parse the output
	flags := strings.TrimSpace(string(output))
	parts := strings.Split(flags, ":")
	if len(parts) != 2 {
		t.Fatalf("Unexpected output format: %s", flags)
	}

	portStr := parts[1]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Failed to parse port number: %v", err)
	}

	// Verify that the port is valid and open
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		t.Fatalf("Port is not open: %v", err)
	}
	ln.Close()
}
