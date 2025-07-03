package main

import (
	"fmt"
	"os"
)

func loadScannedSHAs(filename string) (map[string]struct{}, error) {
	file, err := os.Open(filename)
	if os.IsNotExist(err) {
		return make(map[string]struct{}), nil // start fresh
	} else if err != nil {
		return nil, err
	}
	defer file.Close()

	scanned := make(map[string]struct{})
	var sha string
	for {
		_, err := fmt.Fscanf(file, "%s\n", &sha)
		if err != nil {
			break
		}
		scanned[sha] = struct{}{}
	}
	return scanned, nil
}

func appendScannedSHA(filename, sha string) error {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(sha + "\n")
	return err
}

// Truncate long strings
func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
