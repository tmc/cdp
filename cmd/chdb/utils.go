package main

import (
	"net/url"
	"strings"
)

// urlToLocalPath converts a URL to a local file path
func urlToLocalPath(resourceURL string) (string, error) {
	u, err := url.Parse(resourceURL)
	if err != nil {
		return "", err
	}

	// Basic path cleaning
	path := u.Hostname() + u.Path
	if u.Path == "" || u.Path == "/" {
		path = u.Hostname() + "/index.html" // Guess?
	}
	// Ensure no leading slash for txtar/local fs
	path = strings.TrimPrefix(path, "/")

	return path, nil
}
