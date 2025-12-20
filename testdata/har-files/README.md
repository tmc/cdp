# HAR Files Test Data

This directory contains HAR (HTTP Archive) files and related network capture data, organized by date.

## Structure

```
testdata/har-files/
├── YYYY-MM-DD/          # Date-organized HAR files
│   └── *.har            # HAR capture files
│   └── *.proxymanlogv2  # Proxyman log files
└── README.md            # This file
```

## File Naming Convention

HAR files follow this naming convention:
```
<domain>_MM-DD-YYYY-HH-MM-SS.har
```

For example:
- `generativelanguage.googleapis.com_10-20-2025-00-44-10.har`

## Contents

### 2025-10-19
- Google Generative Language API captures

### 2025-10-20
- Google Generative Language API captures
- Proxyman log files for comparative analysis

## Usage

These files are used for:
- Testing HAR parsing functionality
- Validating cookie handling behavior
- Comparing network capture implementations
- Regression testing
