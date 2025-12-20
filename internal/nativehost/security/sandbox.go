package security

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SandboxConfig holds sandbox configuration
type SandboxConfig struct {
	AllowedCommands []string
	MaxExecutionTime time.Duration
	MaxOutputSize   int
	WorkingDir      string
	Environment     map[string]string
}

// DefaultSandboxConfig returns default sandbox configuration
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		AllowedCommands:  []string{}, // No commands allowed by default
		MaxExecutionTime: 30 * time.Second,
		MaxOutputSize:    1024 * 1024, // 1MB
		WorkingDir:       "/tmp",
		Environment:      map[string]string{},
	}
}

// Sandbox provides sandboxed command execution
type Sandbox struct {
	config SandboxConfig
	validator *Validator
}

// NewSandbox creates a new sandbox
func NewSandbox(config SandboxConfig) *Sandbox {
	return &Sandbox{
		config:    config,
		validator: NewValidator(StrictValidationConfig()),
	}
}

// ExecuteResult holds command execution results
type ExecuteResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
	Truncated bool
}

// Execute runs a command in the sandbox
func (s *Sandbox) Execute(ctx context.Context, cmdStr string, args []string) (*ExecuteResult, error) {
	// Validate command
	if err := s.validator.ValidateCommand(cmdStr); err != nil {
		return nil, fmt.Errorf("command validation failed: %w", err)
	}

	// Check if command is in allowed list
	if len(s.config.AllowedCommands) > 0 {
		allowed := false
		for _, allowedCmd := range s.config.AllowedCommands {
			if cmdStr == allowedCmd || strings.HasPrefix(cmdStr, allowedCmd+" ") {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("command not in allowed list: %s", cmdStr)
		}
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, s.config.MaxExecutionTime)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(execCtx, cmdStr, args...)
	cmd.Dir = s.config.WorkingDir

	// Set environment
	if len(s.config.Environment) > 0 {
		env := make([]string, 0, len(s.config.Environment))
		for k, v := range s.config.Environment {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}

	// Execute and capture output
	start := time.Now()
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	result := &ExecuteResult{
		Duration: duration,
	}

	// Check for truncation
	if len(output) > s.config.MaxOutputSize {
		result.Truncated = true
		output = output[:s.config.MaxOutputSize]
	}

	result.Stdout = string(output)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return result, fmt.Errorf("command execution failed: %w", err)
		}
	}

	return result, nil
}

// IsCommandAllowed checks if a command is allowed
func (s *Sandbox) IsCommandAllowed(cmd string) bool {
	if len(s.config.AllowedCommands) == 0 {
		return false
	}

	for _, allowedCmd := range s.config.AllowedCommands {
		if cmd == allowedCmd || strings.HasPrefix(cmd, allowedCmd+" ") {
			return true
		}
	}

	return false
}
