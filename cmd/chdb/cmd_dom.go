package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/spf13/cobra"
)

// DOM manipulation commands
var domCmd = &cobra.Command{
	Use:   "dom",
	Short: "DOM manipulation and inspection",
	Long:  "Query, modify, and inspect DOM elements",
}

var domGetCmd = &cobra.Command{
	Use:   "get <selector>",
	Short: "Get element details by selector",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := getElement(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to get element: %v", err)
		}
	},
}

var domSetCmd = &cobra.Command{
	Use:   "set <selector> <attribute> <value>",
	Short: "Set element attribute",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := setElementAttribute(ctx, args[0], args[1], args[2], tabID); err != nil {
			log.Fatalf("Failed to set attribute: %v", err)
		}
	},
}

var domRemoveCmd = &cobra.Command{
	Use:   "remove <selector>",
	Short: "Remove element from DOM",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := removeElement(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to remove element: %v", err)
		}
	},
}

var domHighlightCmd = &cobra.Command{
	Use:   "highlight <selector>",
	Short: "Highlight element on page",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")
		duration, _ := cmd.Flags().GetDuration("duration")

		if err := highlightElement(ctx, args[0], tabID, duration); err != nil {
			log.Fatalf("Failed to highlight element: %v", err)
		}
	},
}

var domTreeCmd = &cobra.Command{
	Use:   "tree [selector]",
	Short: "Get DOM tree structure",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")
		depth, _ := cmd.Flags().GetInt("depth")

		selector := "body"
		if len(args) > 0 {
			selector = args[0]
		}

		if err := getDOMTree(ctx, selector, depth, tabID); err != nil {
			log.Fatalf("Failed to get DOM tree: %v", err)
		}
	},
}

var domHtmlCmd = &cobra.Command{
	Use:   "html <selector>",
	Short: "Get outer HTML of element",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := getOuterHTML(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to get HTML: %v", err)
		}
	},
}

var domBoxCmd = &cobra.Command{
	Use:   "box <selector>",
	Short: "Get box model of element",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := createContext()
		tabID, _ := cmd.Flags().GetString("tab")

		if err := getBoxModel(ctx, args[0], tabID); err != nil {
			log.Fatalf("Failed to get box model: %v", err)
		}
	},
}

func init() {
	// Add subcommands to dom command
	domCmd.AddCommand(domGetCmd)
	domCmd.AddCommand(domSetCmd)
	domCmd.AddCommand(domRemoveCmd)
	domCmd.AddCommand(domHighlightCmd)
	domCmd.AddCommand(domTreeCmd)
	domCmd.AddCommand(domHtmlCmd)
	domCmd.AddCommand(domBoxCmd)

	// Add flags
	domGetCmd.Flags().String("tab", "", "Target tab ID")
	domSetCmd.Flags().String("tab", "", "Target tab ID")
	domRemoveCmd.Flags().String("tab", "", "Target tab ID")
	domHighlightCmd.Flags().String("tab", "", "Target tab ID")
	domHighlightCmd.Flags().Duration("duration", 3*time.Second, "Highlight duration")
	domTreeCmd.Flags().String("tab", "", "Target tab ID")
	domTreeCmd.Flags().Int("depth", 3, "Tree depth to display")
	domHtmlCmd.Flags().String("tab", "", "Target tab ID")
	domBoxCmd.Flags().String("tab", "", "Target tab ID")
}

// Implementation functions

func getElement(ctx context.Context, selector string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create DOM controller
	domCtrl := NewDOMController(debugger, verbose)

	// Query element
	node, err := domCtrl.QuerySelector(ctx, selector)
	if err != nil {
		return err
	}

	// Get attributes
	attrs, err := domCtrl.GetAttributes(ctx, selector)
	if err != nil {
		log.Printf("Warning: Could not get attributes: %v", err)
	}

	fmt.Printf("Element found: %s\n", selector)
	fmt.Printf("  NodeID: %d\n", node.NodeID)
	fmt.Printf("  NodeType: %d\n", node.NodeType)
	fmt.Printf("  NodeName: %s\n", node.NodeName)
	if node.NodeValue != "" {
		fmt.Printf("  NodeValue: %s\n", node.NodeValue)
	}

	if len(attrs) > 0 {
		fmt.Println("  Attributes:")
		for name, value := range attrs {
			fmt.Printf("    %s: %s\n", name, value)
		}
	}

	return nil
}

func setElementAttribute(ctx context.Context, selector string, attribute string, value string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create DOM controller
	domCtrl := NewDOMController(debugger, verbose)

	// Set attribute
	if err := domCtrl.SetAttribute(ctx, selector, attribute, value); err != nil {
		return err
	}

	fmt.Printf("✓ Set attribute '%s' to '%s' on element: %s\n", attribute, value, selector)

	return nil
}

func removeElement(ctx context.Context, selector string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create DOM controller
	domCtrl := NewDOMController(debugger, verbose)

	// Remove node
	if err := domCtrl.RemoveNode(ctx, selector); err != nil {
		return err
	}

	fmt.Printf("✓ Removed element: %s\n", selector)

	return nil
}

func highlightElement(ctx context.Context, selector string, tabID string, duration time.Duration) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create DOM controller
	domCtrl := NewDOMController(debugger, verbose)

	// Highlight node
	if err := domCtrl.HighlightNode(ctx, selector); err != nil {
		return err
	}

	fmt.Printf("✓ Highlighting element: %s\n", selector)

	// Keep highlight for duration
	time.Sleep(duration)

	// Hide highlight
	if err := domCtrl.HideHighlight(ctx); err != nil {
		log.Printf("Warning: Could not hide highlight: %v", err)
	}

	return nil
}

func getDOMTree(ctx context.Context, selector string, depth int, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// For now, use JavaScript to get DOM tree
	expression := fmt.Sprintf(`
		(function() {
			const el = document.querySelector('%s');
			if (!el) return null;

			function buildTree(node, maxDepth, currentDepth = 0) {
				if (currentDepth >= maxDepth) return null;

				const tree = {
					nodeName: node.nodeName,
					nodeType: node.nodeType,
					id: node.id || undefined,
					className: node.className || undefined,
					childCount: node.childNodes.length,
					children: []
				};

				if (node.childNodes.length > 0 && currentDepth < maxDepth - 1) {
					tree.children = Array.from(node.childNodes)
						.filter(n => n.nodeType === 1) // Elements only
						.slice(0, 10) // Limit children
						.map(n => buildTree(n, maxDepth, currentDepth + 1));
				}

				return tree;
			}

			return buildTree(el, %d);
		})()
	`, selector, depth)

	result, err := debugger.Execute(ctx, expression)
	if err != nil {
		return err
	}

	// Pretty print the tree
	if treeJSON, err := json.MarshalIndent(result, "", "  "); err == nil {
		fmt.Printf("DOM Tree for: %s\n", selector)
		fmt.Println(string(treeJSON))
	} else {
		fmt.Printf("DOM Tree: %v\n", result)
	}

	return nil
}

func getOuterHTML(ctx context.Context, selector string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create DOM controller
	domCtrl := NewDOMController(debugger, verbose)

	// Get outer HTML
	html, err := domCtrl.GetOuterHTML(ctx, selector)
	if err != nil {
		return err
	}

	fmt.Printf("Outer HTML for: %s\n", selector)
	fmt.Println(html)

	return nil
}

func getBoxModel(ctx context.Context, selector string, tabID string) error {
	debugger := NewChromeDebugger(port, verbose)
	defer debugger.Close()

	// Connect to target
	if err := debugger.Connect(ctx, tabID); err != nil {
		return err
	}

	// Create DOM controller
	domCtrl := NewDOMController(debugger, verbose)

	// Get box model
	model, err := domCtrl.GetBoxModel(ctx, selector)
	if err != nil {
		return err
	}

	fmt.Printf("Box Model for: %s\n", selector)
	fmt.Printf("  Content: %v\n", model.Content)
	fmt.Printf("  Padding: %v\n", model.Padding)
	fmt.Printf("  Border: %v\n", model.Border)
	fmt.Printf("  Margin: %v\n", model.Margin)
	fmt.Printf("  Width: %d\n", model.Width)
	fmt.Printf("  Height: %d\n", model.Height)

	return nil
}