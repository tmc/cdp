package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

// CSS manipulation commands
var cssCmd = &cobra.Command{
	Use:   "css",
	Short: "CSS inspection and manipulation",
	Long:  "Query, modify, and analyze CSS styles and rules",
}

var cssRulesCmd = &cobra.Command{
	Use:   "rules <selector>",
	Short: "Get CSS rules for element",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := getCSSRules(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to get CSS rules: %v", err)
		}
	},
}

var cssComputedCmd = &cobra.Command{
	Use:   "computed <selector>",
	Short: "Get computed styles for element",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := getComputedStyles(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to get computed styles: %v", err)
		}
	},
}

var cssSetCmd = &cobra.Command{
	Use:   "set <selector> <property> <value>",
	Short: "Set inline style property",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := setCSSProperty(ctx, args[0], args[1], args[2], tabID); err != nil {
			log.Fatalf("Failed to set CSS property: %v", err)
		}
	},
}

var cssInlineCmd = &cobra.Command{
	Use:   "inline <selector>",
	Short: "Get inline styles for element",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := getInlineStyles(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to get inline styles: %v", err)
		}
	},
}

var cssAddRuleCmd = &cobra.Command{
	Use:   "add-rule <rule>",
	Short: "Add CSS rule to document",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := addCSSRule(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to add CSS rule: %v", err)
		}
	},
}

var cssCoverageCmd = &cobra.Command{
	Use:   "coverage",
	Short: "Get CSS coverage data",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := getCSSCoverage(ctx, tabID); err != nil {
			log.Fatalf("Failed to get CSS coverage: %v", err)
		}
	},
}

var cssListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all stylesheets",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := listStylesheets(ctx, tabID); err != nil {
			log.Fatalf("Failed to list stylesheets: %v", err)
		}
	},
}

func init() {
	// Add subcommands to css command
	cssCmd.AddCommand(cssRulesCmd)
	cssCmd.AddCommand(cssComputedCmd)
	cssCmd.AddCommand(cssSetCmd)
	cssCmd.AddCommand(cssInlineCmd)
	cssCmd.AddCommand(cssAddRuleCmd)
	cssCmd.AddCommand(cssCoverageCmd)
	cssCmd.AddCommand(cssListCmd)

	// Add flags
	cssRulesCmd.Flags().String("tab", "", "Target tab ID")
	cssComputedCmd.Flags().String("tab", "", "Target tab ID")
	cssSetCmd.Flags().String("tab", "", "Target tab ID")
	cssInlineCmd.Flags().String("tab", "", "Target tab ID")
	cssAddRuleCmd.Flags().String("tab", "", "Target tab ID")
	cssCoverageCmd.Flags().String("tab", "", "Target tab ID")
	cssListCmd.Flags().String("tab", "", "Target tab ID")
}

// Implementation functions

func getCSSRules(ctx context.Context, selector string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create CSS controller
	cssCtrl := NewCSSController(debugger, verbose)

	// Get matched styles
	result, err := cssCtrl.GetMatchedStyles(ctx, selector)
	if err != nil {
		return err
	}

	fmt.Printf("CSS Rules for: %s\n", selector)
	fmt.Println("===============================")

	// Display the result as JSON for now
	if resultJSON, err := json.MarshalIndent(result, "", "  "); err == nil {
		fmt.Println(string(resultJSON))
	} else {
		fmt.Printf("%v\n", result)
	}

	return nil
}

func getComputedStyles(ctx context.Context, selector string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create CSS controller
	cssCtrl := NewCSSController(debugger, verbose)

	// Get computed styles
	styles, err := cssCtrl.GetComputedStyles(ctx, selector)
	if err != nil {
		return err
	}

	fmt.Printf("Computed Styles for: %s\n", selector)
	fmt.Println("================================")

	for property, value := range styles {
		fmt.Printf("  %s: %s\n", property, value)
	}

	return nil
}

func setCSSProperty(ctx context.Context, selector string, property string, value string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create CSS controller
	cssCtrl := NewCSSController(debugger, verbose)

	// Set inline style
	if err := cssCtrl.SetInlineStyle(ctx, selector, property, value); err != nil {
		return err
	}

	fmt.Printf("✓ Set CSS property '%s' to '%s' on element: %s\n", property, value, selector)

	return nil
}

func getInlineStyles(ctx context.Context, selector string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create CSS controller
	cssCtrl := NewCSSController(debugger, verbose)

	// Get inline styles
	styles, err := cssCtrl.GetInlineStyles(ctx, selector)
	if err != nil {
		return err
	}

	fmt.Printf("Inline Styles for: %s\n", selector)
	fmt.Println("========================")

	if len(styles) == 0 {
		fmt.Println("  No inline styles found")
		return nil
	}

	for property, value := range styles {
		fmt.Printf("  %s: %s\n", property, value)
	}

	return nil
}

func addCSSRule(ctx context.Context, ruleText string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create CSS controller
	cssCtrl := NewCSSController(debugger, verbose)

	// Add CSS rule
	if err := cssCtrl.AddCSSRule(ctx, ruleText); err != nil {
		return err
	}

	fmt.Printf("✓ Added CSS rule: %s\n", ruleText)

	return nil
}

func getCSSCoverage(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create CSS controller
	cssCtrl := NewCSSController(debugger, verbose)

	fmt.Println("Starting CSS coverage collection...")

	// Start coverage
	if err := cssCtrl.StartCSSCoverage(ctx); err != nil {
		return err
	}

	fmt.Println("CSS coverage started. Perform some actions on the page, then run this command again to get results.")
	fmt.Println("Note: This is a simplified implementation. In practice, you'd want to:")
	fmt.Println("  1. Start coverage")
	fmt.Println("  2. Perform page interactions")
	fmt.Println("  3. Stop coverage to get results")

	// For demonstration, immediately stop and show results
	coverage, err := cssCtrl.StopCSSCoverage(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("\nCSS Coverage Results (%d rules):\n", len(coverage))
	fmt.Println("===================================")

	for i, rule := range coverage {
		if ruleJSON, err := json.MarshalIndent(rule, "", "  "); err == nil {
			fmt.Printf("[%d] %s\n", i+1, string(ruleJSON))
		} else {
			fmt.Printf("[%d] %v\n", i+1, rule)
		}
	}

	return nil
}

func listStylesheets(ctx context.Context, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Use JavaScript to get stylesheets since CDP doesn't provide direct access
	expression := `
		(function() {
			const sheets = [];
			for (let i = 0; i < document.styleSheets.length; i++) {
				const sheet = document.styleSheets[i];
				const info = {
					index: i,
					href: sheet.href || 'inline',
					title: sheet.title || 'untitled',
					disabled: sheet.disabled,
					type: sheet.type,
					media: sheet.media.mediaText || 'all',
					rulesCount: 0
				};

				try {
					info.rulesCount = sheet.cssRules ? sheet.cssRules.length : 0;
				} catch (e) {
					info.rulesCount = 'inaccessible (CORS)';
				}

				sheets.push(info);
			}
			return sheets;
		})()
	`

	result, err := debugger.Execute(ctx, expression)
	if err != nil {
		return err
	}

	fmt.Println("Stylesheets in Document:")
	fmt.Println("========================")

	// Pretty print the stylesheets
	if sheetsJSON, err := json.MarshalIndent(result, "", "  "); err == nil {
		fmt.Println(string(sheetsJSON))
	} else {
		fmt.Printf("%v\n", result)
	}

	return nil
}