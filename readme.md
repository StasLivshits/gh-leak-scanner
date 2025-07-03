# GitHub Commit Leak Scanner

A Go tool to scan all commits in a GitHub repository for secrets or sensitive information using customizable regular expression rules.

## Features

- Scans all branches and commits in a repository
- Uses regex rules to detect secrets (e.g., AWS keys, Secrets, etc.)
- Avoids re-scanning already processed commits (using `scanned_commits.txt`)
- Supports concurrent scanning with a worker pool
- Handles GitHub API rate limiting
- Outputs findings with commit details

## Requirements

- Go 1.18 or newer
- GitHub access token with `repo` scope

## Usage

Run the scanner with the required flags:

```bash
./gh-leak-scanner -owner <owner> -repo <repo> -token <your_github_token>
```

## Example Output

```text
Found 2 potential leaks in repository 'example/repo':

---
Commit:    0a7f3a1d29d6
File:      config/secrets.go
Rule:      AWS Access Key
Snippet:   "AKIAIOSFODNN7EXAMPLE"
Committer: Jane Doe <jane@example.com>
Date:      2025-07-03T12:44:19Z

---
Commit:    6cd29ac30a2f
File:      main.go
Rule:      TODO Comment
Snippet:   "TODO: remove hardcoded credentials"
Committer: John Dev <john@corp.com>
Date:      2025-07-01T09:13:52Z
```
## Resumable Scanning

This tool supports resumable scans to avoid reprocessing the same commits after a crash or interruption.

- Every successfully scanned commit SHA is recorded in a file called `scanned_commits.txt`.
- On restart, the scanner skips any commit already listed in that file.
- This helps conserve API calls and speeds up incremental scans.

### Resetting the Scan

To force a full rescan from scratch, simply delete the file:

```bash
rm scanned_commits.txt
```

## Regex Rules

The scanner uses customizable regular expressions to detect potential secrets, credentials, or policy violations in commit diffs.

Regex rules are defined directly in the source code:

```go
var regexRules = []RegexRule{
    {
        Name:    "AWS Access Key",
        Pattern: `(?i)(A3T[A-Z0-9]{17}|AKIA[0-9A-Z]{16}|ASIA[0-9A-Z]{16})`,
    },
    {
        Name:    "Secret Access Key",
        Pattern: `(?i)[A-Za-z0-9/+=]{40}`,
    }, 
}
```
These rules can be modified to suit your specific scanning needs. Each rule consists of a name and a regex pattern.


