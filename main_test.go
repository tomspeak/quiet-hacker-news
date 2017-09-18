package main

import (
	"testing"
)

func TestTrimSubdomain(t *testing.T) {
	var urls = []struct {
		url        string
		trimmedURL string
	}{
		{
			url:        "www.quiethn.com",
			trimmedURL: "quiethn.com",
		},
		{
			url:        "www.w3.org",
			trimmedURL: "w3.org",
		},
		{
			url:        "w3.org",
			trimmedURL: "w3.org",
		},
		{
			url:        "http://www.w3.org",
			trimmedURL: "w3.org",
		},
		{
			url:        "https://www.w3.org",
			trimmedURL: "w3.org",
		},
		{
			url:        "http://w3.org",
			trimmedURL: "w3.org",
		},
		{
			url:        "https://w3.org",
			trimmedURL: "w3.org",
		},
	}

	for _, url := range urls {
		st := trimSubdomain(url.url)

		if st != url.trimmedURL {
			t.Errorf("URL was incorrectly trimmed, got: %v, want: %v", st, url.trimmedURL)
		}
	}
}
