package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Performance profiling",
	Long:  "CPU and heap profiling for both Node.js and Chrome",
}

var profileStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start CPU profiling",
	Run: func(cmd *cobra.Command, args []string) {
		callCmd.Run(cmd, []string{"Profiler.start", "{}"})
		fmt.Fprintln(os.Stderr, "Profiler started")
	},
}

var profileStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop CPU profiling and save",
	Run: func(cmd *cobra.Command, args []string) {
		// This needs custom handling to save the profile
		// So we can't just use callCmd directly for the output part
		// Implement specialized logic later or rely on generic output
		callCmd.Run(cmd, []string{"Profiler.stop", "{}"})
	},
}

var profileCPUCmd = &cobra.Command{
	Use:   "cpu [duration]",
	Short: "Start CPU profiling",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		duration := "10s"
		if len(args) > 0 {
			duration = args[0]
		}

		ctx := createContext()
		profiler := NewProfiler(verbose)

		output, _ := cmd.Flags().GetString("output")
		target, _ := cmd.Flags().GetString("target")

		if err := profiler.ProfileCPU(ctx, duration, output, target); err != nil {
			log.Fatalf("Failed to profile CPU: %v", err)
		}
	},
}

var profileHeapCmd = &cobra.Command{
	Use:   "heap",
	Short: "Take heap snapshot",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		profiler := NewProfiler(verbose)

		output, _ := cmd.Flags().GetString("output")
		target, _ := cmd.Flags().GetString("target")

		if err := profiler.ProfileHeap(ctx, output, target); err != nil {
			log.Fatalf("Failed to profile heap: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(profileCmd)

	profileCmd.AddCommand(profileStartCmd)
	profileCmd.AddCommand(profileStopCmd)
	profileCmd.AddCommand(profileCPUCmd)
	profileCmd.AddCommand(profileHeapCmd)

	profileCPUCmd.Flags().StringP("output", "o", "cpu-profile.json", "Output file")
	profileCPUCmd.Flags().StringP("target", "t", "", "Target ID")

	profileHeapCmd.Flags().StringP("output", "o", "heap-snapshot.json", "Output file")
	profileHeapCmd.Flags().StringP("target", "t", "", "Target ID")
}
