#!/bin/bash
# Example usage of computer-use-agent

# Make sure GEMINI_API_KEY is set
if [ -z "$GEMINI_API_KEY" ]; then
    echo "Error: GEMINI_API_KEY environment variable is not set"
    echo "Get your API key from: https://aistudio.google.com/app/apikey"
    exit 1
fi

# Path to the binary
AGENT="./bin/computer-use-agent"

# Example 1: Basic search with Chrome (default)
echo "Example 1: Basic search"
$AGENT --query "Search Google for 'Go 1.22 features' and tell me the top 3 features"

# Example 2: Using Brave Browser
echo -e "\nExample 2: Using Brave Browser"
$AGENT \
  --chrome-path "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser" \
  --query "Search for the latest AI news"

# Example 3: Using a specific profile with cookies/session
echo -e "\nExample 3: Using existing profile"
$AGENT \
  --profile "Default" \
  --query "Check my GitHub notifications"

# Example 4: Headless mode for scripting
echo -e "\nExample 4: Headless mode"
$AGENT \
  --headless \
  --query "What is the current temperature in San Francisco according to Google?"

# Example 5: Form automation
echo -e "\nExample 5: Form automation"
$AGENT \
  --initial-url "https://www.google.com/search?q=contact+form+example" \
  --query "Find a contact form on the first result and fill it with test data"

# Example 6: Verbose mode for debugging
echo -e "\nExample 6: Verbose debugging"
$AGENT \
  --verbose \
  --query "Navigate to news.ycombinator.com and tell me the top story"

# Example 7: Research task with longer timeout
echo -e "\nExample 7: Complex research task"
$AGENT \
  --timeout 600 \
  --query "Compare the pricing of the top 3 cloud providers (AWS, GCP, Azure) for basic compute instances"

# Example 8: Using with Brave and a profile (authenticated sessions)
echo -e "\nExample 8: Brave with profile for authenticated tasks"
$AGENT \
  --chrome-path "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser" \
  --profile "Profile 1" \
  --query "Go to my Google Calendar and tell me what meetings I have today"

# Example 9: Testing a web application
echo -e "\nExample 9: Web app testing"
$AGENT \
  --initial-url "https://example.com/app" \
  --query "Test the search functionality by searching for 'test' and verify results appear"

# Example 10: Data extraction
echo -e "\nExample 10: Extract structured data"
$AGENT \
  --query "Go to news.ycombinator.com and create a JSON list of the top 5 posts with title, score, and URL"
