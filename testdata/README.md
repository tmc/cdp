# Chrome-to-HAR Test Data

This directory contains test data and analysis documentation for the chrome-to-har project.

## Directory Structure

```
testdata/
├── analysis/              # Analysis documentation organized by date
│   ├── 2025-10-20/       # October 20, 2025 analysis documents
│   │   ├── ultrathinking-analysis-2025-10-20.md
│   │   ├── all-fixes-summary.md
│   │   ├── latest-har-analysis.md
│   │   ├── successful-go-run-analysis.md
│   │   ├── go-python-comparison-final.md
│   │   └── final-fix-summary.md
│   └── README.md
├── har-files/            # HAR captures and network logs organized by date
│   ├── 2025-10-19/      # October 19, 2025 captures
│   ├── 2025-10-20/      # October 20, 2025 captures
│   └── README.md
└── README.md            # This file
```

## Contents

### Analysis Documentation
Located in `analysis/`, organized by date. Contains:
- Technical analysis documents
- Fix summaries
- Comparison reports
- Session notes

See `analysis/README.md` for details.

### HAR Files
Located in `har-files/`, organized by date. Contains:
- HAR (HTTP Archive) format network captures
- Proxyman log files
- Network traffic recordings

See `har-files/README.md` for details.

## Organization Policy

- **Date-based organization**: All files are organized into subdirectories by date (YYYY-MM-DD format)
- **Permanence**: Files moved from `/tmp/` to ensure they are not lost
- **Documentation**: Each subdirectory contains a README explaining its contents
- **Binary files**: Not committed to git, only stored locally for testing

## Usage

These files are used for:
- Development and debugging
- Test case creation
- Regression testing
- Performance analysis
- Documentation and reference
