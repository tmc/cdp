// Package htmltomd converts HTML to Markdown.
package htmltomd

import (
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/JohannesKaufmann/html-to-markdown/plugin"
)

// Convert converts an HTML string to GitHub-flavored Markdown.
func Convert(html string) (string, error) {
	conv := md.NewConverter("", true, nil)
	conv.Use(plugin.GitHubFlavored())
	return conv.ConvertString(html)
}

// ConvertDomain converts HTML to Markdown, resolving relative links against domain.
func ConvertDomain(html, domain string) (string, error) {
	conv := md.NewConverter(domain, true, nil)
	conv.Use(plugin.GitHubFlavored())
	result, err := conv.ConvertString(html)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result), nil
}
