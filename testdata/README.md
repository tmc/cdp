# CDP Test Data

This directory contains committed test fixtures for the repository.

## Directory Structure

```
testdata/
├── analysis/             # Notes about analysis-related fixture layout
├── cdp/                  # Small CDP-oriented source and sourcemap fixtures
├── har-files/            # HAR captures and network logs organized by date
│   └── README.md
└── README.md            # This file
```

## Contents

### CDP Fixtures
Located in `cdp/`. Contains:
- Small JavaScript breakpoint fixtures
- Tiny TypeScript/source map samples

### HAR Files
Located in `har-files/`, organized by date. Contains:
- HAR (HTTP Archive) format network captures
- Proxyman log files
- Network traffic recordings

See `har-files/README.md` for details.

## Usage

These files are used for:
- Development and debugging
- Test case creation
- Regression testing
- Performance analysis
- Documentation and reference
