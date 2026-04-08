#!/usr/bin/env node

const inspector = require('inspector');
const fs = require('fs');
const path = require('path');
const { exec, execSync } = require('child_process');
const util = require('util');
const execPromise = util.promisify(exec);

class ClaudeDebugger {
  constructor() {
    this.sessions = new Map();
    this.debugPort = 9229;
  }

  async findClaudeProcesses() {
    try {
      const { stdout } = await execPromise('ps aux | grep -i claude | grep -v grep');
      const processes = stdout.split('\n').filter(line => line.trim()).map(line => {
        const parts = line.split(/\s+/);
        return {
          user: parts[0],
          pid: parseInt(parts[1]),
          cpu: parseFloat(parts[2]),
          mem: parseFloat(parts[3]),
          command: parts.slice(10).join(' ')
        };
      });
      return processes;
    } catch (error) {
      console.error('Error finding Claude processes:', error);
      return [];
    }
  }

  async findNodeProcesses() {
    try {
      const { stdout } = await execPromise('ps aux | grep node | grep -v grep');
      const processes = stdout.split('\n').filter(line => line.trim()).map(line => {
        const parts = line.split(/\s+/);
        return {
          user: parts[0],
          pid: parseInt(parts[1]),
          cpu: parseFloat(parts[2]),
          mem: parseFloat(parts[3]),
          command: parts.slice(10).join(' ')
        };
      });
      return processes;
    } catch (error) {
      console.error('Error finding Node processes:', error);
      return [];
    }
  }

  async enableDebuggerForPID(pid, port = null) {
    const debugPort = port || this.debugPort++;
    console.log(`Attempting to enable debugger for PID ${pid} on port ${debugPort}`);

    try {
      // Send SIGUSR1 to enable inspector (for Node.js processes)
      execSync(`kill -USR1 ${pid}`);
      console.log(`Sent SIGUSR1 to PID ${pid} - inspector should be enabled`);

      // Give it a moment to start
      await new Promise(resolve => setTimeout(resolve, 1000));

      // Check if inspector is listening
      const { stdout } = await execPromise(`lsof -p ${pid} | grep LISTEN | grep 9229`);
      if (stdout) {
        console.log(`Inspector is listening for PID ${pid}:`, stdout.trim());
        return debugPort;
      }
    } catch (error) {
      console.error(`Failed to enable debugger for PID ${pid}:`, error.message);
    }
    return null;
  }

  async getStackTrace(pid) {
    console.log(`Getting stack trace for PID ${pid}`);
    try {
      // Use lldb on macOS to get stack trace
      const script = `
        attach ${pid}
        bt all
        detach
        quit
      `;

      const { stdout } = await execPromise(`echo '${script}' | lldb`, { maxBuffer: 10 * 1024 * 1024 });
      return stdout;
    } catch (error) {
      console.error(`Failed to get stack trace for PID ${pid}:`, error.message);

      // Try alternative with sample command (macOS)
      try {
        const { stdout } = await execPromise(`sample ${pid} 1 -f`, { maxBuffer: 10 * 1024 * 1024 });
        return stdout;
      } catch (sampleError) {
        console.error('Sample command also failed:', sampleError.message);
      }
    }
    return null;
  }

  async getFileDescriptors(pid) {
    console.log(`Getting file descriptors for PID ${pid}`);
    try {
      const { stdout } = await execPromise(`lsof -p ${pid}`);
      return stdout;
    } catch (error) {
      console.error(`Failed to get file descriptors for PID ${pid}:`, error.message);
      return null;
    }
  }

  async getProcessInfo(pid) {
    console.log(`Getting detailed process info for PID ${pid}`);
    const info = {
      pid,
      timestamp: new Date().toISOString()
    };

    try {
      // Basic process info
      const { stdout: psInfo } = await execPromise(`ps -p ${pid} -o pid,ppid,user,state,pcpu,pmem,vsz,rss,etime,command`);
      info.processInfo = psInfo;

      // CPU and memory usage
      const { stdout: topInfo } = await execPromise(`top -pid ${pid} -l 1 -stats pid,cpu,mem,time,state`);
      info.resourceUsage = topInfo;

      // Thread count
      try {
        const { stdout: threadInfo } = await execPromise(`ps -M ${pid} | wc -l`);
        info.threadCount = parseInt(threadInfo.trim()) - 1;
      } catch (e) {}

      // Network connections
      try {
        const { stdout: netInfo } = await execPromise(`lsof -p ${pid} -i`);
        info.networkConnections = netInfo;
      } catch (e) {}

    } catch (error) {
      console.error('Error getting process info:', error.message);
    }

    return info;
  }

  async diagnoseStuckProcess(pid) {
    console.log(`\n${'='.repeat(50)}`);
    console.log(`Diagnosing potentially stuck process: PID ${pid}`);
    console.log(`${'='.repeat(50)}\n`);

    const diagnosis = {
      pid,
      timestamp: new Date().toISOString(),
      issues: []
    };

    // 1. Get basic process info
    console.log('1. Collecting process information...');
    const processInfo = await this.getProcessInfo(pid);
    diagnosis.processInfo = processInfo;

    // 2. Check CPU usage
    console.log('2. Analyzing CPU usage...');
    const cpuCheck = await this.checkCPUUsage(pid);
    if (cpuCheck.isHigh) {
      diagnosis.issues.push({
        type: 'HIGH_CPU',
        description: 'Process is consuming high CPU',
        details: cpuCheck
      });
    } else if (cpuCheck.isZero) {
      diagnosis.issues.push({
        type: 'NO_CPU_ACTIVITY',
        description: 'Process shows no CPU activity (might be deadlocked)',
        details: cpuCheck
      });
    }

    // 3. Get stack trace
    console.log('3. Capturing stack trace...');
    const stackTrace = await this.getStackTrace(pid);
    diagnosis.stackTrace = stackTrace;

    // 4. Check file descriptors
    console.log('4. Analyzing file descriptors...');
    const fileDescriptors = await this.getFileDescriptors(pid);
    diagnosis.fileDescriptors = fileDescriptors;

    // 5. Try to enable Node debugger
    console.log('5. Attempting to enable Node.js debugger...');
    const debugPort = await this.enableDebuggerForPID(pid);
    if (debugPort) {
      diagnosis.debuggerEnabled = true;
      diagnosis.debugPort = debugPort;
      console.log(`\nDebugger enabled! Connect with: chrome://inspect or node inspect localhost:${debugPort}`);
    }

    // 6. Analyze potential issues
    if (stackTrace) {
      if (stackTrace.includes('poll') || stackTrace.includes('select') || stackTrace.includes('kevent')) {
        diagnosis.issues.push({
          type: 'BLOCKED_IO',
          description: 'Process appears to be blocked on I/O operation',
          recommendation: 'Check network connections and file operations'
        });
      }
      if (stackTrace.includes('mutex') || stackTrace.includes('lock') || stackTrace.includes('semaphore')) {
        diagnosis.issues.push({
          type: 'POTENTIAL_DEADLOCK',
          description: 'Process might be in a deadlock situation',
          recommendation: 'Review thread synchronization and locking code'
        });
      }
    }

    return diagnosis;
  }

  async checkCPUUsage(pid) {
    try {
      const { stdout } = await execPromise(`ps -p ${pid} -o pcpu=`);
      const cpu = parseFloat(stdout.trim());
      return {
        cpu,
        isHigh: cpu > 80,
        isZero: cpu < 0.1
      };
    } catch (error) {
      return { cpu: null, isHigh: false, isZero: false };
    }
  }

  async monitorProcess(pid, duration = 10000) {
    console.log(`Monitoring PID ${pid} for ${duration/1000} seconds...`);
    const startTime = Date.now();
    const samples = [];

    const interval = setInterval(async () => {
      const cpuUsage = await this.checkCPUUsage(pid);
      samples.push({
        timestamp: Date.now() - startTime,
        cpu: cpuUsage.cpu
      });
    }, 1000);

    await new Promise(resolve => setTimeout(resolve, duration));
    clearInterval(interval);

    // Analyze samples
    const avgCpu = samples.reduce((sum, s) => sum + (s.cpu || 0), 0) / samples.length;
    const maxCpu = Math.max(...samples.map(s => s.cpu || 0));
    const minCpu = Math.min(...samples.map(s => s.cpu || 0));

    return {
      samples,
      statistics: {
        average: avgCpu.toFixed(2),
        max: maxCpu.toFixed(2),
        min: minCpu.toFixed(2),
        variance: this.calculateVariance(samples.map(s => s.cpu || 0)).toFixed(2)
      }
    };
  }

  calculateVariance(values) {
    const mean = values.reduce((sum, val) => sum + val, 0) / values.length;
    const squaredDiffs = values.map(val => Math.pow(val - mean, 2));
    return squaredDiffs.reduce((sum, val) => sum + val, 0) / values.length;
  }

  async generateReport(pid, outputFile = null) {
    console.log(`\nGenerating comprehensive diagnostic report for PID ${pid}...`);

    const report = {
      metadata: {
        timestamp: new Date().toISOString(),
        pid,
        platform: process.platform,
        nodeVersion: process.version
      },
      diagnosis: await this.diagnoseStuckProcess(pid),
      monitoring: await this.monitorProcess(pid, 5000)
    };

    const reportText = this.formatReport(report);

    if (outputFile) {
      fs.writeFileSync(outputFile, reportText);
      console.log(`\nReport saved to: ${outputFile}`);
    }

    return report;
  }

  formatReport(report) {
    let text = `
CLAUDE CODE PROCESS DIAGNOSTIC REPORT
======================================
Generated: ${report.metadata.timestamp}
PID: ${report.metadata.pid}
Platform: ${report.metadata.platform}
Node Version: ${report.metadata.nodeVersion}

ISSUES DETECTED
---------------`;

    if (report.diagnosis.issues.length === 0) {
      text += '\nNo obvious issues detected.';
    } else {
      report.diagnosis.issues.forEach(issue => {
        text += `\n\n* ${issue.type}: ${issue.description}`;
        if (issue.recommendation) {
          text += `\n  Recommendation: ${issue.recommendation}`;
        }
      });
    }

    text += `\n\nCPU MONITORING (5 seconds)
--------------------------
Average CPU: ${report.monitoring.statistics.average}%
Max CPU: ${report.monitoring.statistics.max}%
Min CPU: ${report.monitoring.statistics.min}%
Variance: ${report.monitoring.statistics.variance}

PROCESS INFORMATION
-------------------
${report.diagnosis.processInfo.processInfo || 'Not available'}
`;

    if (report.diagnosis.debuggerEnabled) {
      text += `\nDEBUGGER ACCESS
---------------
Debugger enabled on port: ${report.diagnosis.debugPort}
Connect with: chrome://inspect or node inspect localhost:${report.diagnosis.debugPort}
`;
    }

    return text;
  }
}

// CLI Interface
async function main() {
  const debugger = new ClaudeDebugger();
  const args = process.argv.slice(2);

  if (args.length === 0 || args[0] === '--help') {
    console.log(`
Claude Code Node.js Debugger Utility
Usage: node nodejs-claude-debugger.js [command] [options]

Commands:
  list              - List all Claude and Node processes
  diagnose <pid>    - Diagnose a potentially stuck process
  monitor <pid>     - Monitor a process for 10 seconds
  debug <pid>       - Enable Node.js debugger for a process
  report <pid>      - Generate comprehensive diagnostic report

Options:
  --output <file>   - Save report to file
  --duration <ms>   - Monitoring duration (default: 10000ms)

Examples:
  node nodejs-claude-debugger.js list
  node nodejs-claude-debugger.js diagnose 12345
  node nodejs-claude-debugger.js report 12345 --output report.txt
`);
    return;
  }

  const command = args[0];

  switch (command) {
    case 'list':
      console.log('\nClaude Processes:');
      const claudeProcs = await debugger.findClaudeProcesses();
      claudeProcs.forEach(p => {
        console.log(`  PID ${p.pid}: CPU ${p.cpu}% MEM ${p.mem}% - ${p.command.substring(0, 80)}`);
      });

      console.log('\nNode.js Processes:');
      const nodeProcs = await debugger.findNodeProcesses();
      nodeProcs.forEach(p => {
        console.log(`  PID ${p.pid}: CPU ${p.cpu}% MEM ${p.mem}% - ${p.command.substring(0, 80)}`);
      });
      break;

    case 'diagnose':
      if (args[1]) {
        const pid = parseInt(args[1]);
        const diagnosis = await debugger.diagnoseStuckProcess(pid);
        console.log('\nDiagnosis Results:');
        console.log(JSON.stringify(diagnosis, null, 2));
      } else {
        console.error('Please provide a PID');
      }
      break;

    case 'monitor':
      if (args[1]) {
        const pid = parseInt(args[1]);
        const duration = args.includes('--duration') ?
          parseInt(args[args.indexOf('--duration') + 1]) : 10000;
        const results = await debugger.monitorProcess(pid, duration);
        console.log('\nMonitoring Results:');
        console.log(JSON.stringify(results.statistics, null, 2));
      } else {
        console.error('Please provide a PID');
      }
      break;

    case 'debug':
      if (args[1]) {
        const pid = parseInt(args[1]);
        await debugger.enableDebuggerForPID(pid);
      } else {
        console.error('Please provide a PID');
      }
      break;

    case 'report':
      if (args[1]) {
        const pid = parseInt(args[1]);
        const outputFile = args.includes('--output') ?
          args[args.indexOf('--output') + 1] : null;
        await debugger.generateReport(pid, outputFile);
      } else {
        console.error('Please provide a PID');
      }
      break;

    default:
      console.error(`Unknown command: ${command}`);
  }
}

// Export for use as module
module.exports = ClaudeDebugger;

// Run if executed directly
if (require.main === module) {
  main().catch(console.error);
}