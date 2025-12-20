# Computer Use Agent

AI-powered browser automation using Google Gemini's computer use capabilities.

## Overview

This tool uses Google's Gemini AI model to control a Chrome/Brave browser via natural language commands. It implements Gemini's computer use API to automate web interactions through an AI agent.

## Installation

```bash
cd /Users/tmc/go/src/github.com/tmc/misc/chrome-to-har
go build -o bin/computer-use-agent ./cmd/computer-use-agent
```

## Usage

### Basic Example

```bash
export GEMINI_API_KEY="your-api-key-here"
./bin/computer-use-agent --query "Search Google for 'Go 1.22 features' and summarize the top result"
```

### Using Brave Browser

```bash
# On macOS
export CHROME_PATH="/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"
./bin/computer-use-agent --query "Find the latest news about AI"

# Or use the flag
./bin/computer-use-agent \
  --chrome-path "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser" \
  --query "Search for Rust programming tutorials"
```

### Advanced Examples

```bash
# Fill a web form
./bin/computer-use-agent \
  --initial-url "https://example.com/contact" \
  --query "Fill in the contact form with name 'John Doe' and email 'john@example.com'"

# Research task with verbose output
./bin/computer-use-agent \
  --query "Find and compare the prices of the top 3 laptops on Amazon" \
  --verbose

# Headless mode (no browser window)
./bin/computer-use-agent \
  --headless \
  --query "Check if google.com is accessible and report the page title"
```

## Configuration

### Environment Variables

- `GEMINI_API_KEY` - Google Gemini API key (required)
- `CHROME_PATH` - Path to Chrome or Brave executable (optional)

### Command Line Flags

```
--query string            Natural language task description (required unless --shell)
--model string            Gemini model name (default: "gemini-2.0-flash-exp")
--initial-url string      Starting URL (default: "https://www.google.com")
--headless               Run browser in headless mode
--verbose                Enable verbose logging
--timeout int            Timeout in seconds (default: 300)
--chrome-path string     Path to Chrome/Brave executable
--profile string         Chrome/Brave profile name (e.g., "Default", "Profile 1")
--har-output string      Output HAR file path (optional)
--shell                  Interactive shell mode - allows multi-turn conversations
--debug-port int         Chrome DevTools debugging port (0 = auto-assign)
--devtools               Automatically open Chrome DevTools in browser window
--use-os-screenshots     Use macOS screen-capture for full window capture (includes DevTools)
```

## Browser Compatibility

This tool works with both Google Chrome and Brave Browser. To use Brave, set the `CHROME_PATH` environment variable or use the `--chrome-path` flag to point to your Brave executable.

### Finding Your Browser Executable

**macOS:**
- Chrome: `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`
- Brave: `/Applications/Brave Browser.app/Contents/MacOS/Brave Browser`

**Linux:**
- Chrome: `/usr/bin/google-chrome` or `/usr/bin/google-chrome-stable`
- Brave: `/usr/bin/brave-browser` or `/opt/brave.com/brave/brave`

**Windows:**
- Chrome: `C:\Program Files\Google\Chrome\Application\chrome.exe`
- Brave: `C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`

## How It Works

1. **Browser Launch**: Launches Chrome/Brave with CDP (Chrome DevTools Protocol) enabled
2. **Initial Navigation**: Navigates to the specified initial URL
3. **AI Agent Loop**:
   - Sends the current page state (screenshot + URL) to Gemini
   - Gemini analyzes the page and decides what actions to take
   - Actions are executed on the browser (click, type, scroll, navigate, etc.)
   - Loop continues until the task is complete or timeout is reached

## Available Browser Actions

The agent can perform these actions:

- **Navigation**: `navigate`, `go_back`, `go_forward`, `search`
- **Mouse**: `click_at`, `hover_at`, `drag_and_drop`
- **Keyboard**: `type_text_at`, `key_combination`
- **Scrolling**: `scroll_document`, `scroll_at`
- **Utility**: `wait_5_seconds`

All coordinates are normalized to a 0-1000 scale for consistency across different screen sizes.

## Interactive Shell Mode

The agent supports an interactive shell mode that allows multi-turn conversations:

```bash
./bin/computer-use-agent --shell
```

In shell mode:
- The browser stays open between commands
- Conversation history is maintained across turns
- The AI remembers context from previous interactions
- Type `exit` or `quit` to end the session

Example session:

```
> Navigate to example.com
Agent: I have navigated to example.com.
✅ Done

> What is the main heading?
Agent: The main heading on this page is "Example Domain".
✅ Done

> Go to Wikipedia
Agent: I have navigated to Wikipedia.
✅ Done

> exit
👋 Goodbye!
```

### Shell Mode with Initial Query

You can also provide an initial query that runs before entering shell mode:

```bash
./bin/computer-use-agent --shell --query "Navigate to GitHub and search for 'go'"
```

## Examples

### Research Task

```bash
./bin/computer-use-agent --query "Search for 'quantum computing breakthroughs 2024' and summarize the findings"
```

### Form Automation

```bash
./bin/computer-use-agent \
  --initial-url "https://forms.example.com" \
  --query "Fill out the survey with positive feedback"
```

### Web Scraping

```bash
./bin/computer-use-agent \
  --query "Go to news.ycombinator.com and list the top 5 stories with their scores"
```

### Testing

```bash
./bin/computer-use-agent \
  --initial-url "https://example.com/app" \
  --query "Test the login flow with username 'test@example.com' and verify successful login"
```

### HAR Recording

Record network traffic and agent actions to a HAR file:

```bash
./bin/computer-use-agent \
  --query "Navigate to example.com and check the page content" \
  --har-output /tmp/session.har
```

The HAR file will include:
- All network requests and responses
- Automatic annotations for agent actions (navigate, click, type)
- Screenshots at key moments
- Timestamps for each action

## DevTools Integration

The agent supports Chrome DevTools integration for debugging and development:

### Auto-Open DevTools

Automatically open DevTools in the browser window:

```bash
./bin/computer-use-agent \
  --devtools \
  --query "Navigate to example.com and inspect the page"
```

**Note**: DevTools opens docked by default. Press `Ctrl+Shift+D` in the browser to undock it to a separate window for better visibility.

### Remote DevTools Debugging

Enable remote DevTools debugging on a specific port:

```bash
./bin/computer-use-agent \
  --debug-port 9222 \
  --query "Your task here"
```

Then open `http://localhost:9222` in Chrome to inspect the browser session remotely.

### Full Window Screenshots (Including DevTools)

By default, screenshots are captured via Chrome DevTools Protocol (CDP), which only captures the page content, not the browser UI or DevTools panel.

To capture the entire browser window including DevTools, use OS-level screenshots:

```bash
./bin/computer-use-agent \
  --devtools \
  --use-os-screenshots \
  --query "Debug this webpage"
```

**Requirements for `--use-os-screenshots`:**
- macOS only
- Requires `screen-capture` tool (install with `go install github.com/tmc/macgo/examples/screen-capture@latest`)
- Requires `list-app-windows` tool (install with `go install github.com/tmc/macgo/examples/list-app-windows@latest`)
- Requires `mouse-move` tool for smooth mouse movement (install with `go install github.com/tmc/macgo/examples/mouse-move@latest`)
- Requires `mouse-click` tool for clicking on DevTools UI (install with `go install github.com/tmc/macgo/examples/mouse-click@latest`)
- Requires Screen Recording permission (will be prompted on first use)
- Requires Accessibility permission for `mouse-click` and `mouse-move` to control the mouse (will be prompted on first use)

**How it works:**
1. Detects the Chrome/Brave window ID using `list-app-windows`
2. Uses the `screen-capture` tool to capture the entire window (including DevTools panel!)
3. Each `screen-capture` subprocess runs in its own macgo bundle with independent TCC permissions
4. When clicking with OS screenshots enabled:
   - Gets window bounds from `list-app-windows`
   - Converts normalized coordinates (0-1000) to absolute screen coordinates
   - Smoothly moves mouse to target using `mouse-move` tool with natural human-like movement
   - Performs OS-level mouse click using `mouse-click` tool (CGEvent APIs) with visual indicator
   - Falls back to CDP clicks (viewport-relative) if OS click fails
5. Falls back to CDP screenshots if screen capture fails

**Benefits:**
- ✅ Captures the **entire browser window** including DevTools panel
- ✅ AI can see Network tab, Console, Elements panel, etc.
- ✅ Can click on DevTools UI elements when dependencies are installed
- ✅ **Natural human-like mouse movement** with Bezier curves and ease-in/ease-out
- ✅ **Visual click indicators** show exactly where the AI is clicking
- ✅ Perfect for debugging and inspecting web applications
- ✅ Works reliably with macgo's unique pipe directory per invocation
- ✅ Graceful fallback to CDP for both screenshots and clicks

**Clicking Behavior:**
- With `--use-os-screenshots`: Uses OS-level clicks at absolute screen coordinates (can click DevTools UI)
- Without flag: Uses CDP clicks at viewport-relative coordinates (page content only)
- Automatically falls back to CDP if OS click requirements aren't met

This is particularly useful when:
- You want the AI to see and interact with DevTools
- You need to debug complex web applications
- You want visual confirmation that DevTools is open and showing the right information
- You want to click on browser UI elements like DevTools tabs

## Troubleshooting

### Browser Not Found

If you get an error about Chrome not being found:
1. Install Chrome or Brave browser
2. Set the `CHROME_PATH` environment variable to the executable path
3. Or use the `--chrome-path` flag

### API Key Issues

Make sure your `GEMINI_API_KEY` is set correctly:
```bash
export GEMINI_API_KEY="your-key-here"
echo $GEMINI_API_KEY  # Verify it's set
```

### Timeout Errors

If tasks are timing out, increase the timeout:
```bash
./bin/computer-use-agent --timeout 600 --query "your long-running task"
```

## Limitations

- The agent operates in a loop with a maximum number of turns (default: 50)
- Screenshot context is limited to the last 3 turns to manage token usage
- Complex multi-page workflows may require breaking into smaller tasks
- The AI may not always interpret page layouts perfectly

## License

See the main chrome-to-har project LICENSE file.

## Credits

Built on top of the [chrome-to-har](https://github.com/tmc/misc/chrome-to-har) browser automation library.
