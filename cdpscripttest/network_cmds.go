package cdpscripttest

import (
	"context"
	"fmt"
	"strconv"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"rsc.io/script"
)

// NetworkCmds returns commands for network condition emulation.
func NetworkCmds() map[string]script.Cmd {
	return map[string]script.Cmd{
		"network-emulate":       NetworkEmulate(),
		"network-emulate-clear": NetworkEmulateClear(),
	}
}

// NetworkEmulate returns a command that activates network condition emulation
// via the CDP Network domain. It supports both HTTP throttling (latency,
// throughput) and WebRTC packet emulation (loss, queue, reordering).
//
// Usage: network-emulate [--loss N] [--queue N] [--reorder] [--latency N] [--down N] [--up N]
func NetworkEmulate() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "emulate network conditions (packet loss, latency, throughput)",
			Args:    "[--loss N] [--queue N] [--reorder] [--latency N] [--down N] [--up N]",
			Detail: []string{
				"--loss N       WebRTC packet loss percent (0-100)",
				"--queue N      WebRTC packet queue length",
				"--reorder      enable WebRTC packet reordering",
				"--latency N    HTTP latency in ms",
				"--down N       HTTP download throughput bytes/sec (-1 = no limit)",
				"--up N         HTTP upload throughput bytes/sec (-1 = no limit)",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			var (
				loss    float64
				queue   int64
				reorder bool
				latency float64
				down    float64 = -1
				up      float64 = -1
			)
			hasLoss := false
			hasQueue := false

			for i := 0; i < len(args); i++ {
				switch args[i] {
				case "--loss":
					i++
					if i >= len(args) {
						return nil, fmt.Errorf("--loss requires a value")
					}
					v, err := strconv.ParseFloat(args[i], 64)
					if err != nil {
						return nil, fmt.Errorf("--loss: %w", err)
					}
					loss = v
					hasLoss = true
				case "--queue":
					i++
					if i >= len(args) {
						return nil, fmt.Errorf("--queue requires a value")
					}
					v, err := strconv.ParseInt(args[i], 10, 64)
					if err != nil {
						return nil, fmt.Errorf("--queue: %w", err)
					}
					queue = v
					hasQueue = true
				case "--reorder":
					reorder = true
				case "--latency":
					i++
					if i >= len(args) {
						return nil, fmt.Errorf("--latency requires a value")
					}
					v, err := strconv.ParseFloat(args[i], 64)
					if err != nil {
						return nil, fmt.Errorf("--latency: %w", err)
					}
					latency = v
				case "--down":
					i++
					if i >= len(args) {
						return nil, fmt.Errorf("--down requires a value")
					}
					v, err := strconv.ParseFloat(args[i], 64)
					if err != nil {
						return nil, fmt.Errorf("--down: %w", err)
					}
					down = v
				case "--up":
					i++
					if i >= len(args) {
						return nil, fmt.Errorf("--up requires a value")
					}
					v, err := strconv.ParseFloat(args[i], 64)
					if err != nil {
						return nil, fmt.Errorf("--up: %w", err)
					}
					up = v
				default:
					return nil, fmt.Errorf("unknown flag %q", args[i])
				}
			}

			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}

			return func(s *script.State) (stdout, stderr string, err error) {
				conditions := &network.Conditions{
					Latency:            latency,
					DownloadThroughput: down,
					UploadThroughput:   up,
					PacketReordering:   reorder,
				}
				if hasLoss {
					conditions.PacketLoss = loss
				}
				if hasQueue {
					conditions.PacketQueueLength = queue
				}
				err = chromedp.Run(cs.cdpCtx, chromedp.ActionFunc(func(ctx context.Context) error {
					if _, err := network.EmulateNetworkConditionsByRule(false, []*network.Conditions{conditions}).Do(ctx); err != nil {
						return err
					}
					return network.OverrideNetworkState(false, latency, down, up).Do(ctx)
				}))
				if err != nil {
					return "", "", err
				}
				return "", "", nil
			}, nil
		},
	)
}

// NetworkEmulateClear returns a command that resets all network emulation.
//
// Usage: network-emulate-clear
func NetworkEmulateClear() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "reset all network condition emulation",
			Args:    "",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 0 {
				return nil, script.ErrUsage
			}
			cs, err := cdpState(s)
			if err != nil {
				return nil, err
			}
			return func(s *script.State) (stdout, stderr string, err error) {
				err = chromedp.Run(cs.cdpCtx, chromedp.ActionFunc(func(ctx context.Context) error {
					reset := &network.Conditions{
						DownloadThroughput: -1,
						UploadThroughput:   -1,
					}
					if _, err := network.EmulateNetworkConditionsByRule(false, []*network.Conditions{reset}).Do(ctx); err != nil {
						return err
					}
					return network.OverrideNetworkState(false, 0, -1, -1).Do(ctx)
				}))
				if err != nil {
					return "", "", err
				}
				return "", "", nil
			}, nil
		},
	)
}
