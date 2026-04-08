#!/bin/bash

# Claude Code Debug Attachment Script
# Helps attach debugger to stuck Claude Code sessions

set -euo pipefail

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

print_header() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  Claude Code Debug Attachment Tool${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo
}

find_claude_processes() {
    echo -e "${YELLOW}Finding Claude Code processes...${NC}"
    ps aux | grep -i claude | grep -v grep | while read -r line; do
        pid=$(echo "$line" | awk '{print $2}')
        cpu=$(echo "$line" | awk '{print $3}')
        mem=$(echo "$line" | awk '{print $4}')
        cmd=$(echo "$line" | awk '{for(i=11;i<=NF;i++) printf "%s ", $i; print ""}')

        echo -e "${GREEN}PID:${NC} $pid ${GREEN}CPU:${NC} ${cpu}% ${GREEN}MEM:${NC} ${mem}%"
        echo -e "  ${GREEN}CMD:${NC} ${cmd:0:80}"

        # Check if process appears stuck (low CPU for Claude usually means stuck)
        if (( $(echo "$cpu < 1.0" | bc -l) )); then
            echo -e "  ${YELLOW}⚠ Warning: Low CPU usage - might be stuck${NC}"
        fi
        echo
    done
}

attach_debugger() {
    local pid=$1
    echo -e "${YELLOW}Attempting to attach debugger to PID $pid...${NC}"

    # Try to send SIGUSR1 to enable inspector (works for Node.js processes)
    echo -e "${BLUE}Sending SIGUSR1 signal to enable inspector...${NC}"
    if kill -USR1 "$pid" 2>/dev/null; then
        echo -e "${GREEN}✓ SIGUSR1 sent successfully${NC}"
        sleep 1

        # Check if inspector is listening
        if lsof -p "$pid" 2>/dev/null | grep -q "9229"; then
            echo -e "${GREEN}✓ Inspector is listening on port 9229${NC}"
            echo
            echo -e "${GREEN}Connect to debugger using one of these methods:${NC}"
            echo -e "  1. Chrome DevTools: ${BLUE}chrome://inspect${NC}"
            echo -e "  2. Node Inspector: ${BLUE}node inspect localhost:9229${NC}"
            echo -e "  3. VS Code: Attach to Node Process (port 9229)"
            return 0
        else
            echo -e "${YELLOW}⚠ Inspector not detected on default port${NC}"
        fi
    else
        echo -e "${RED}✗ Failed to send SIGUSR1 (process might not be Node.js)${NC}"
    fi

    return 1
}

get_stack_trace() {
    local pid=$1
    echo -e "${YELLOW}Getting stack trace for PID $pid...${NC}"

    if command -v sample &> /dev/null; then
        echo -e "${BLUE}Using 'sample' command (macOS)...${NC}"
        sample "$pid" 1 -f 2>/dev/null | head -100
    elif command -v gdb &> /dev/null; then
        echo -e "${BLUE}Using GDB...${NC}"
        echo -e "thread apply all bt\ndetach\nquit" | gdb -p "$pid" 2>/dev/null | grep -A 20 "Thread"
    elif command -v lldb &> /dev/null; then
        echo -e "${BLUE}Using LLDB...${NC}"
        echo -e "bt all\ndetach\nquit" | lldb -p "$pid" 2>/dev/null | grep -A 20 "thread"
    else
        echo -e "${RED}No suitable debugger found (sample/gdb/lldb)${NC}"
        return 1
    fi
}

analyze_process() {
    local pid=$1
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  Analyzing Process PID: $pid${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo

    # Basic info
    echo -e "${YELLOW}Process Information:${NC}"
    ps -p "$pid" -o pid,ppid,user,state,pcpu,pmem,vsz,rss,etime,command 2>/dev/null || {
        echo -e "${RED}Process $pid not found${NC}"
        return 1
    }
    echo

    # File descriptors
    echo -e "${YELLOW}Open File Descriptors:${NC}"
    lsof -p "$pid" 2>/dev/null | head -20 || echo "Unable to get file descriptors"
    echo

    # Network connections
    echo -e "${YELLOW}Network Connections:${NC}"
    lsof -p "$pid" -i 2>/dev/null || echo "No network connections found"
    echo

    # Thread count
    echo -e "${YELLOW}Thread Information:${NC}"
    ps -M "$pid" 2>/dev/null | wc -l | xargs -I {} echo "Thread count: {}"
    echo

    # Try to attach debugger
    if attach_debugger "$pid"; then
        echo -e "${GREEN}✓ Debugger attached successfully${NC}"
    else
        echo -e "${YELLOW}Falling back to stack trace analysis...${NC}"
        get_stack_trace "$pid"
    fi
}

monitor_process() {
    local pid=$1
    local duration=${2:-10}

    echo -e "${YELLOW}Monitoring PID $pid for ${duration} seconds...${NC}"
    echo -e "${BLUE}Time\tCPU%\tMEM%\tSTATE${NC}"

    for ((i=0; i<duration; i++)); do
        stats=$(ps -p "$pid" -o pcpu=,pmem=,state= 2>/dev/null || echo "- - -")
        echo -e "$(date +%H:%M:%S)\t$stats"
        sleep 1
    done

    echo
    echo -e "${GREEN}Monitoring complete${NC}"
}

inject_code() {
    local pid=$1
    echo -e "${YELLOW}Preparing code injection for PID $pid...${NC}"

    cat << 'EOF' > /tmp/claude-inject.js
// Emergency diagnostic code
const fs = require('fs');
const util = require('util');

// Capture current state
const diagnostics = {
    timestamp: new Date().toISOString(),
    pid: process.pid,
    memoryUsage: process.memoryUsage(),
    activeHandles: process._getActiveHandles ? process._getActiveHandles().length : 'N/A',
    activeRequests: process._getActiveRequests ? process._getActiveRequests().length : 'N/A',
    eventLoopLag: null
};

// Measure event loop lag
const start = Date.now();
setImmediate(() => {
    diagnostics.eventLoopLag = Date.now() - start;

    // Write diagnostics
    fs.writeFileSync('/tmp/claude-diagnostics-' + process.pid + '.json',
        JSON.stringify(diagnostics, null, 2));

    console.log('Diagnostics written to /tmp/claude-diagnostics-' + process.pid + '.json');
});

// Log to console
console.log('=== CLAUDE DEBUG INJECTION ===');
console.log('PID:', process.pid);
console.log('Memory:', util.inspect(process.memoryUsage()));
console.log('===============================');
EOF

    echo -e "${GREEN}Injection code prepared at /tmp/claude-inject.js${NC}"
    echo -e "${YELLOW}To inject into the process:${NC}"
    echo -e "  1. Attach debugger to PID $pid"
    echo -e "  2. In debugger console: ${BLUE}require('/tmp/claude-inject.js')${NC}"
}

# Main menu
main() {
    print_header

    if [[ $# -eq 0 ]]; then
        echo "Usage: $0 <command> [options]"
        echo
        echo "Commands:"
        echo "  list                - List all Claude processes"
        echo "  analyze <pid>       - Analyze a specific process"
        echo "  attach <pid>        - Attach debugger to process"
        echo "  monitor <pid> [sec] - Monitor process CPU/memory"
        echo "  trace <pid>         - Get stack trace"
        echo "  inject <pid>        - Prepare diagnostic code injection"
        echo "  auto                - Auto-detect and analyze stuck processes"
        echo
        exit 0
    fi

    case "$1" in
        list)
            find_claude_processes
            ;;
        analyze)
            [[ -z "$2" ]] && { echo -e "${RED}Error: PID required${NC}"; exit 1; }
            analyze_process "$2"
            ;;
        attach)
            [[ -z "$2" ]] && { echo -e "${RED}Error: PID required${NC}"; exit 1; }
            attach_debugger "$2"
            ;;
        monitor)
            [[ -z "$2" ]] && { echo -e "${RED}Error: PID required${NC}"; exit 1; }
            monitor_process "$2" "${3:-10}"
            ;;
        trace)
            [[ -z "$2" ]] && { echo -e "${RED}Error: PID required${NC}"; exit 1; }
            get_stack_trace "$2"
            ;;
        inject)
            [[ -z "$2" ]] && { echo -e "${RED}Error: PID required${NC}"; exit 1; }
            inject_code "$2"
            ;;
        auto)
            echo -e "${YELLOW}Auto-detecting stuck Claude processes...${NC}"
            ps aux | grep -i claude | grep -v grep | while read -r line; do
                pid=$(echo "$line" | awk '{print $2}')
                cpu=$(echo "$line" | awk '{print $3}')

                if (( $(echo "$cpu < 1.0" | bc -l) )); then
                    echo -e "${YELLOW}Found potentially stuck process: PID $pid (CPU: ${cpu}%)${NC}"
                    analyze_process "$pid"
                    echo
                fi
            done
            ;;
        *)
            echo -e "${RED}Unknown command: $1${NC}"
            exit 1
            ;;
    esac
}

main "$@"