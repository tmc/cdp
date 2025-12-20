# Computer Use Agent - Quick Start

Get started with AI-powered browser automation in under 5 minutes!

## Prerequisites

1. **Go 1.21+** installed
2. **Google Gemini API Key** - Get one from [Google AI Studio](https://aistudio.google.com/app/apikey)
3. **Chrome or Brave Browser** installed

## Quick Start

### 1. Build the Tool

```bash
cd /Users/tmc/go/src/github.com/tmc/misc/chrome-to-har
go build -o bin/computer-use-agent ./cmd/computer-use-agent
```

### 2. Set Your API Key

```bash
export GEMINI_API_KEY="your-api-key-here"
```

### 3. Run Your First Task

```bash
./bin/computer-use-agent --query "Search Google for 'Golang' and tell me what it is"
```

That's it! The agent will:
- Launch a browser
- Navigate to Google
- Search for "Golang"
- Read the results
- Summarize what Go is

## Using with Brave Browser

### Option 1: Environment Variable

```bash
export CHROME_PATH="/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"
./bin/computer-use-agent --query "your task here"
```

### Option 2: Command Line Flag

```bash
./bin/computer-use-agent \
  --chrome-path "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser" \
  --query "your task here"
```

### Linux Brave Path

```bash
./bin/computer-use-agent \
  --chrome-path "/usr/bin/brave-browser" \
  --query "your task here"
```

## Using an Existing Profile

Use your existing browser profile to access logged-in sessions:

```bash
./bin/computer-use-agent \
  --profile "Default" \
  --query "Check my Gmail inbox and tell me how many unread emails I have"
```

Common profile names:
- `Default` - Your main profile
- `Profile 1`, `Profile 2`, etc. - Additional profiles

## DevTools and Debugging

### Auto-Open DevTools

Have DevTools open automatically for debugging:

```bash
./bin/computer-use-agent \
  --devtools \
  --query "Navigate to example.com and inspect the console"
```

Press `Ctrl+Shift+D` in the browser to undock DevTools to a separate window.

### Full Window Screenshots (Including DevTools)

Capture the entire browser window including DevTools panel:

```bash
./bin/computer-use-agent \
  --devtools \
  --use-os-screenshots \
  --query "Debug this webpage and show me the network tab"
```

**Note**: Requires macOS and additional tools:
```bash
go install github.com/tmc/macgo/examples/screen-capture@latest
go install github.com/tmc/macgo/examples/list-app-windows@latest
```

## Common Use Cases

### 1. Web Research

```bash
./bin/computer-use-agent \
  --query "Research the latest developments in quantum computing and summarize the top 3 breakthroughs"
```

### 2. Form Filling

```bash
./bin/computer-use-agent \
  --initial-url "https://example.com/signup" \
  --query "Fill the signup form with name 'Test User' and email 'test@example.com'"
```

### 3. Price Comparison

```bash
./bin/computer-use-agent \
  --query "Find the current prices of MacBook Pro 14-inch on Amazon, Best Buy, and Apple.com"
```

### 4. News Aggregation

```bash
./bin/computer-use-agent \
  --query "Go to Hacker News and list the top 5 posts with scores > 100"
```

### 5. Website Testing

```bash
./bin/computer-use-agent \
  --initial-url "https://your-app.com" \
  --query "Test the login flow with test credentials and verify it works"
```

## Tips

### Run in Headless Mode

For automation scripts, run without a visible browser:

```bash
./bin/computer-use-agent --headless --query "your task"
```

### Enable Verbose Logging

See what the agent is doing step-by-step:

```bash
./bin/computer-use-agent --verbose --query "your task"
```

### Increase Timeout for Long Tasks

```bash
./bin/computer-use-agent --timeout 600 --query "complex multi-step task"
```

### Start from a Specific URL

```bash
./bin/computer-use-agent \
  --initial-url "https://github.com" \
  --query "Find the most popular Go repositories"
```

## Troubleshooting

### "Browser not found"

Set the path to your browser:

```bash
# macOS Chrome
export CHROME_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"

# macOS Brave
export CHROME_PATH="/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"

# Linux Chrome
export CHROME_PATH="/usr/bin/google-chrome"

# Linux Brave
export CHROME_PATH="/usr/bin/brave-browser"
```

### "API key not set"

Make sure you've exported your Gemini API key:

```bash
export GEMINI_API_KEY="your-key"
echo $GEMINI_API_KEY  # Verify it's set
```

### Task Times Out

Increase the timeout for complex tasks:

```bash
./bin/computer-use-agent --timeout 900 --query "your task"
```

## Next Steps

- Read the full [README.md](README.md) for advanced usage
- Check out [examples.sh](examples.sh) for more examples
- Explore the [chrome-to-har documentation](../../README.md) for browser automation details

## Support

For issues or questions:
- File an issue on GitHub
- Check the verbose logs with `--verbose` flag
- Review the Gemini API documentation
