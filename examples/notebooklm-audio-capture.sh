#!/bin/bash
# Example: Capture NotebookLM audio download using CDP
#
# This script demonstrates how to use CDP to:
# 1. Launch Chrome with your authenticated profile
# 2. Capture network traffic (HAR)
# 3. Monitor for audio file downloads
# 4. Extract audio URLs

set -e

CDP_BIN="${CDP_BIN:-./cmd/cdp/cdp}"
PROFILE="${PROFILE:-Default}"
OUTPUT_DIR="${OUTPUT_DIR:-/tmp/nlm-capture}"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

mkdir -p "$OUTPUT_DIR"

echo -e "${BLUE}📘 NotebookLM Audio Capture with CDP${NC}"
echo ""
echo "This will:"
echo "  1. Launch Chrome with your $PROFILE profile"
echo "  2. Navigate to NotebookLM"
echo "  3. Capture network traffic to HAR file"
echo "  4. Monitor for audio downloads"
echo ""
echo "After the browser opens:"
echo "  - Navigate to your notebook"
echo "  - Play the audio"
echo "  - Ctrl+C when done"
echo ""
read -p "Press Enter to continue..."

# Check if CDP is built
if [ ! -f "$CDP_BIN" ]; then
    echo -e "${BLUE}Building CDP...${NC}"
    go build -o "$CDP_BIN" ./cmd/cdp
fi

# Step 1: Capture HAR with interactive session
HAR_FILE="$OUTPUT_DIR/notebooklm-$(date +%Y%m%d-%H%M%S).har"

echo -e "${GREEN}Step 1: Capturing network traffic...${NC}"
echo "HAR file: $HAR_FILE"
echo ""

"$CDP_BIN" \
    --use-profile "$PROFILE" \
    --har "$HAR_FILE" \
    --url "https://notebooklm.google.com" \
    --interactive

# Step 2: Analyze HAR for audio URLs
echo ""
echo -e "${GREEN}Step 2: Analyzing captured traffic...${NC}"

if [ -f "$HAR_FILE" ]; then
    echo "Audio-related requests found:"
    jq -r '.log.entries[] | select(.request.url | contains("googleusercontent.com") or contains("audio") or contains(".mp3")) | "\(.request.method) \(.request.url)"' "$HAR_FILE" || echo "  (none found)"

    echo ""
    echo "Largest responses (potential audio files):"
    jq -r '.log.entries[] | select(.response.content.size > 100000) | "\(.response.content.size) bytes - \(.request.url)"' "$HAR_FILE" | head -5 || echo "  (none found)"

    echo ""
    echo -e "${BLUE}Full HAR file saved to: $HAR_FILE${NC}"
    echo ""
    echo "To extract audio URLs:"
    echo "  jq -r '.log.entries[].request.url' $HAR_FILE | grep 'googleusercontent.com'"
    echo ""
    echo "To see headers for authenticated requests:"
    echo "  jq '.log.entries[] | select(.request.url | contains(\"googleusercontent.com\")) | .request.headers' $HAR_FILE"
else
    echo "HAR file not found. Did you capture traffic?"
fi
