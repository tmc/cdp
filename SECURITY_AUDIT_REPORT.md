# Security Audit Report
**Date:** 2025-10-17
**Audit Tools:** gosec v2.x, govulncheck v1.1.4
**Scope:** chrome-to-har project (138 files, 53,820 lines)

## Executive Summary

Security scans identified **195 issues** across the codebase:
- **HIGH severity:** 8 unique issues (integer overflows, weak RNG)
- **MEDIUM severity:** 88 issues (file operations, subprocess calls, weak crypto)
- No known vulnerabilities found in dependencies (govulncheck)

## Critical Findings (HIGH Severity)

### 1. Weak Random Number Generator (G404)
**Location:** `cmd/native-host/main.go:90`
**Issue:** Using `math/rand` instead of `crypto/rand` for jitter calculation
**Risk:** Predictable randomness in retry jitter could be exploited
**Recommendation:** Use `crypto/rand` for security-sensitive randomness

```go
// Current (line 90):
jitter := delay * 0.1 * (0.5 - rand.Float64())

// Recommended:
import "crypto/rand"
// Use crypto/rand for security-sensitive random values
```

### 2. Integer Overflow Vulnerabilities (G115)
**Locations:**
- `internal/visual/comparison.go:92` - uint32 → uint8 conversion (color values)
- `internal/browser/stability.go:334` - int → int32 conversion
- `internal/limits/limits.go:446` - int → int32 conversion
- `internal/errors/retry.go:163` - int → uint conversion
- `cmd/native-host/main.go:267` - int → uint32 conversion

**Risk:** Potential overflow when converting between integer types
**Recommendation:** Add range validation before type conversions

## Medium Severity Issues

### 3. File Permission Issues (G306) - 29 occurrences
**Issue:** Files created with overly permissive permissions
**Risk:** Sensitive data exposure through file system
**Recommendation:** Use restrictive permissions (0600) for sensitive files

### 4. Potential File Inclusion Vulnerabilities (G304) - 19 occurrences
**Issue:** File paths constructed from user input without validation
**Risk:** Path traversal attacks
**Recommendation:** Use filepath.Clean() and validate paths

### 5. Subprocess Launched with Variables (G204) - 11 occurrences
**Issue:** Commands constructed with user-controlled input
**Risk:** Command injection vulnerabilities
**Note:** Most are in test utilities but should still validate inputs

### 6. Weak Cryptographic Primitives (G401) - 4 occurrences
**Issue:** Using MD5 hashing
**Risk:** MD5 is cryptographically broken
**Recommendation:** Use SHA-256 for security-sensitive operations
**Context:** May be acceptable for content fingerprinting

## Vulnerability Scan Results (govulncheck)

**Status:** ✅ No known vulnerabilities found in dependencies

## Code Review Observations

### Positive Security Features
1. ✅ Security manager with HMAC authentication
2. ✅ Rate limiting implementation
3. ✅ Audit logging functionality
4. ✅ Capability-based permissions
5. ✅ Input validation framework
6. ✅ Secure I/O operations module

### Areas for Improvement
1. Default secrets in native-host (line 168)
2. Integer overflow checks needed
3. Path validation could be centralized
4. Weak RNG for security-sensitive operations

## Recommendations by Priority

### High Priority
1. Replace `math/rand` with `crypto/rand` for security operations
2. Add overflow checks for integer type conversions
3. Validate all user-provided file paths
4. Enforce fail-safe for default secrets in production

### Medium Priority
1. Use restrictive file permissions (0600) for sensitive files
2. Upgrade MD5 to SHA-256 or document non-security use
3. Add input validation for subprocess commands
4. Set TLS MinVersion to 1.2+

### Low Priority
1. Review file permission grants
2. Add security-focused tests
3. Consider security linter in CI/CD
4. Document threat model

## Conclusion

The codebase demonstrates good security practices with dedicated security modules. The identified issues are mostly standard Go security anti-patterns that can be addressed systematically. No critical exploitable vulnerabilities were found.

**Overall Risk Level:** MEDIUM
**Recommended Timeline:**
- HIGH severity: 1-2 weeks
- MEDIUM severity: 1-2 months
- LOW priority: Next major release

## Appendix

- gosec report: `gosec-report.json` (195 issues)
- govulncheck report: `govulncheck-main.json` (clean)
- Files scanned: 138
- Lines scanned: 53,820
