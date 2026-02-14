package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chromedp/cdproto/audits"
	"github.com/chromedp/chromedp"
	"github.com/spf13/cobra"
)

var auditDuration time.Duration

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Check for page issues (Audits)",
	Long:  `Enables the Audits domain and reports issues (e.g., Mixed Content, Cookie warnings, etc.) detected on the page.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := runAudit(ctx, tabID); err != nil {
			log.Fatalf("Audit failed: %v", err)
		}
	},
}

func init() {
	auditCmd.Flags().String("tab", "", "Target tab ID")
	auditCmd.Flags().DurationVar(&auditDuration, "duration", 5*time.Second, "Duration to listen for issues")
}

func runAudit(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	issues := make([]*audits.InspectorIssue, 0)

	// Listen for issues
	chromedp.ListenTarget(debugger.chromeCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *audits.EventIssueAdded:
			issues = append(issues, e.Issue)
		}
	})

	log.Printf("Running audit for %v...", auditDuration)

	// Enable Audits domain
	if err := chromedp.Run(debugger.chromeCtx, audits.Enable()); err != nil {
		return fmt.Errorf("failed to enable audits: %w", err)
	}

	select {
	case <-time.After(auditDuration):
	case <-ctx.Done():
		return ctx.Err()
	}

	log.Printf("Audit complete. Found %d issues.", len(issues))

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(issues); err != nil {
		return fmt.Errorf("failed to encode output: %w", err)
	}

	return nil
}
