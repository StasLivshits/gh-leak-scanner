package main

import (
	"context"
	"flag"
	"github.com/google/go-github/v62/github"
	"golang.org/x/oauth2"
	"log"
	"os"
	"regexp"
	"sync"
	"time"
)

const (
	maxWorkers      = 5                     // Number of concurrent workers
	scannedFileName = "scanned_commits.txt" // File to store scanned commit SHAs
)

// RegexRule defines a struct to hold pattern name, raw pattern, and compiled regex
type RegexRule struct {
	Name    string
	Pattern string
	Regex   *regexp.Regexp
}

type LeakFinding struct {
	CommitSHA   string
	FileName    string
	Committer   string
	Date        time.Time
	RuleName    string
	MatchString string
}

// List of regex rules
var regexRules = []RegexRule{
	//{
	//	Name:    "Access Key ID",
	//	Pattern: `(?i)(A3T[A-Z0-9]{17}|AKIA[0-9A-Z]{16}|ASIA[0-9A-Z]{16})`,
	//},
	//{
	//	Name:    "Secret Access Key",
	//	Pattern: `(?i)[A-Za-z0-9/+=]{40}`,
	//	//Pattern: `(?<![A-Za-z0-9/+=])[A-Za-z0-9/+=]{40}(?![A-Za-z0-9/+=])`,
	//},
	// for teting purposes, this will match any line containing TODO
	{
		Name:    "TODO Comment",
		Pattern: `(?i)\bTODO\b.*`, // Matches lines starting with or containing TODO
	},
}

func init() {
	// Compile all regex patterns during initialization
	for i := range regexRules {
		regexRules[i].Regex = regexp.MustCompile(regexRules[i].Pattern)
	}
}

func lookForLeaks(patch string, sha string, fileName string, committer string, date time.Time) []LeakFinding {
	var results []LeakFinding

	for _, rule := range regexRules {
		matches := rule.Regex.FindAllString(patch, -1)
		for _, match := range matches {
			results = append(results, LeakFinding{
				CommitSHA:   sha,
				FileName:    fileName,
				Committer:   committer,
				Date:        date,
				RuleName:    rule.Name,
				MatchString: truncate(match, 80),
			})
		}
	}
	return results
}

func worker(ctx context.Context, client *github.Client, commitChan <-chan string, leakChan chan<- LeakFinding, wg *sync.WaitGroup, repoOwner string, repoName string, rl *RateLimiter) {
	defer wg.Done()
	for sha := range commitChan {
		// check rate limit before each commit fetch
		rl.Check()

		// get full commit data including files
		detailedCommit, _, err := client.Repositories.GetCommit(ctx, repoOwner, repoName, sha, nil)
		if err != nil {
			log.Printf("Failed to fetch commit %s: %v", sha, err)
			continue
		}

		committer := detailedCommit.Commit.Committer.GetName() + " <" + detailedCommit.Commit.Committer.GetEmail() + ">"
		commitDate := detailedCommit.Commit.Committer.GetDate().Time

		// check each file's patch for the regex patterns
		for _, file := range detailedCommit.Files {
			patch := file.GetPatch()
			findings := lookForLeaks(patch, sha, file.GetFilename(), committer, commitDate)
			for _, finding := range findings {
				leakChan <- finding
			}
		}
		// mark this SHA as scanned
		if err := appendScannedSHA(scannedFileName, sha); err != nil {
			log.Printf("Failed to record scanned SHA %s: %v", sha, err)
		}
	}
}

func main() {
	repoOwner := flag.String("owner", "", "GitHub repository owner")
	repoName := flag.String("repo", "", "GitHub repository name")
	accessToken := flag.String("token", "", "GitHub access token")
	flag.Parse()

	if *repoOwner == "" || *repoName == "" || *accessToken == "" {
		log.Fatalln("All parameters -owner, -repo and -token are required")
		os.Exit(1)
	}

	if *accessToken == "" {
		log.Fatalln("access token is required")
		os.Exit(1)
	}

	// authenticate with GitHub using the access token
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: *accessToken})
	tc := oauth2.NewClient(ctx, ts)

	// create a new GitHub client
	client := github.NewClient(tc)

	// create a rate limiter to manage API calls
	rl := NewRateLimiter(ctx, client, 10) // check rate every 10 API calls

	// configure worker pool
	commitChan := make(chan string, 100)
	leakChan := make(chan LeakFinding, 1000)

	var wg sync.WaitGroup

	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go worker(ctx, client, commitChan, leakChan, &wg, *repoOwner, *repoName, rl)
	}

	// get all branches
	branches, _, err := client.Repositories.ListBranches(ctx, *repoOwner, *repoName, &github.BranchListOptions{})
	if err != nil {
		log.Fatalf("Error listing branches: %v", err)
	}

	// load previously scanned commit SHAs for de-duplication and to avoid reprocessing
	scannedSHAs, err := loadScannedSHAs("scanned_commits.txt")
	if err != nil {
		log.Fatalf("Failed to load scanned SHAs: %v", err)
	} else {
		log.Printf("Loaded %d previously scanned commit SHAs", len(scannedSHAs))
	}

	// Iterate through each branch and fetch commits
	for _, branch := range branches {
		branchSha := branch.GetCommit().GetSHA()

		// configure list and pagination options
		listOptions := &github.CommitsListOptions{
			SHA:         branchSha,
			ListOptions: github.ListOptions{PerPage: 100},
		}

		log.Printf("Scanning branch: %s", branch.GetName())

		// start fetching commits
		for {
			// Check rate limit before making API calls
			rl.Check()

			// Fetch commits for the current branch
			// The GitHub API already lists commits in reverse chronological order
			commits, resp, err := client.Repositories.ListCommits(ctx, *repoOwner, *repoName, listOptions)
			if err != nil {
				log.Fatalf("Error fetching commits: %v", err)
			}

			if len(commits) == 0 {
				break
			}

			// Iterate through each commit
			for _, commit := range commits {
				commitSha := commit.GetSHA()
				// Check if commit SHA is already processed
				if _, exists := scannedSHAs[commitSha]; !exists {
					scannedSHAs[commitSha] = struct{}{}
					commitChan <- commitSha
				}
			}

			// Check if there are more pages
			if resp.NextPage == 0 {
				break
			}
			listOptions.Page = resp.NextPage
		}
	}
	close(commitChan)
	wg.Wait()
	close(leakChan)

	// TODO: I dont like this, but it is the simplest way to collect results from multiple goroutines
	// I should process results in the in separate goroutine to prevent overloading a channel, but this is a quick solution
	// Collect and print all findings
	var findings []LeakFinding
	for finding := range leakChan {
		findings = append(findings, finding)
	}

	if len(findings) == 0 {
		log.Println("No leaks found.")
		return
	}
	log.Printf("Found %d potential leaks in the repository %s/%s", len(findings), *repoOwner, *repoName)
	for _, f := range findings {
		log.Printf(`---
			Commit:    %s
			File:      %s
			Rule:      %s
			Snippet:   %q
			Committer: %s
			Date:      %s
	
			`, f.CommitSHA, f.FileName, f.RuleName, f.MatchString, f.Committer, f.Date.Format(time.RFC3339))
	}
}
