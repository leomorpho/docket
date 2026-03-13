package git

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

type Annotation struct {
	TicketID string
	FilePath string
	LineNum  int
	Context  string
}

var annotationRe = regexp.MustCompile(`\[(TKT-\d+)\]`)

// ScanAnnotations walks dir and returns all [TKT-NNN] annotations found.
func ScanAnnotations(repoRoot string) ([]Annotation, error) {
	var results []Annotation

	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return relErr
		}

		if d.IsDir() {
			switch rel {
			case ".git", ".docket", "vendor", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}

		if isSkippedPath(rel) {
			return nil
		}

		isBinary, err := isBinaryFile(path)
		if err != nil {
			return err
		}
		if isBinary {
			return nil
		}

		anns, err := scanFile(repoRoot, path)
		if err != nil {
			return err
		}
		results = append(results, anns...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return results, nil
}

func scanFile(repoRoot, path string) ([]Annotation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return nil, err
	}

	var out []Annotation
	s := bufio.NewScanner(f)
	line := 0
	for s.Scan() {
		line++
		text := s.Text()
		matches := annotationRe.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			out = append(out, Annotation{
				TicketID: m[1],
				FilePath: filepath.ToSlash(rel),
				LineNum:  line,
				Context:  text,
			})
		}
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("scanning %s: %w", path, err)
	}

	return out, nil
}

func isBinaryFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return false, err
	}
	return bytes.IndexByte(buf[:n], 0) >= 0, nil
}

func isSkippedPath(rel string) bool {
	prefixes := []string{".git/", ".docket/", "vendor/", "node_modules/", ".svelte-kit/", "build/", "dist/"}
	for _, p := range prefixes {
		if len(rel) >= len(p) && rel[:len(p)] == p {
			return true
		}
	}
	return false
}
