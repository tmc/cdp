#!/bin/bash

# Speech Agent Control Script
# Provides manual control over the background speech agent

SPEECH_SESSION_ID="10A1C134-756B-439E-8330-E81FF293E5EA"
LOG_FILE="/tmp/speech-agent.log"

show_usage() {
    echo "Usage: $0 [command]"
    echo "Commands:"
    echo "  status    - Check if speech agent is running"
    echo "  announce  - Trigger immediate announcement"
    echo "  stop      - Stop the speech agent"
    echo "  restart   - Restart the speech agent"
    echo "  log       - Show recent log entries"
    echo "  session   - Show session buffer"
}

check_status() {
    if pgrep -f "speech-agent.sh" > /dev/null; then
        echo "✅ Speech agent is running (PID: $(pgrep -f speech-agent.sh))"
        echo "📊 Log entries: $(wc -l < "$LOG_FILE" 2>/dev/null || echo "0")"
        echo "🕐 Last announcement: $(tail -1 "$LOG_FILE" 2>/dev/null | cut -d']' -f1 | tr -d '[' || echo "None")"
    else
        echo "❌ Speech agent is not running"
    fi
}

trigger_announcement() {
    echo "🔊 Triggering immediate announcement..."
    say "Investigation update: ChromeDP debugging session in progress. Key finding confirmed - Node.js debugging requires direct WebSocket approach without Target methods."
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Manual announcement triggered" >> "$LOG_FILE"
}

stop_agent() {
    echo "🛑 Stopping speech agent..."
    pkill -f "speech-agent.sh"
    it2 session send-key "$SPEECH_SESSION_ID" ctrl-c 2>/dev/null || true
    echo "Speech agent stopped"
}

restart_agent() {
    echo "🔄 Restarting speech agent..."
    stop_agent
    sleep 2
    it2 session send-text --send-cr "$SPEECH_SESSION_ID" "./speech-agent.sh"
    echo "Speech agent restarted"
}

show_log() {
    echo "📋 Recent speech agent log entries:"
    echo "═══════════════════════════════════════"
    tail -10 "$LOG_FILE" 2>/dev/null || echo "No log entries found"
}

show_session() {
    echo "📺 Speech agent session buffer:"
    echo "═══════════════════════════════════════"
    it2 text get-buffer "$SPEECH_SESSION_ID" 2>/dev/null | tail -15 || echo "Cannot access session buffer"
}

case "${1:-status}" in
    "status")
        check_status
        ;;
    "announce")
        trigger_announcement
        ;;
    "stop")
        stop_agent
        ;;
    "restart")
        restart_agent
        ;;
    "log")
        show_log
        ;;
    "session")
        show_session
        ;;
    "help"|"-h"|"--help")
        show_usage
        ;;
    *)
        echo "Unknown command: $1"
        show_usage
        exit 1
        ;;
esac