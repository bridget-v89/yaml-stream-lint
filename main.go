package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	maxLineLength := flag.Int("max-line-length", 120, "max allowed line length, 0 disables the check")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [flags] [file ...]\n\nreads from stdin if no files are given.\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	opts := Options{MaxLineLength: *maxLineLength}

	files := flag.Args()
	if len(files) == 0 {
		files = []string{"-"}
	}

	found := false
	for _, path := range files {
		n, err := lintFile(path, opts)
		found = found || n > 0
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(2)
		}
	}

	if found {
		os.Exit(1)
	}
}

func lintFile(path string, opts Options) (int, error) {
	f := os.Stdin
	if path != "-" {
		opened, err := os.Open(path)
		if err != nil {
			return 0, err
		}
		defer opened.Close()
		f = opened
	}

	count := 0
	err := Lint(f, opts, func(finding Finding) {
		count++
		fmt.Printf("%s:%s\n", path, finding)
	})
	return count, err
}
