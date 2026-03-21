package workspace

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Repo struct {
	Name string
	Path string
}

func Discover(root string) ([]Repo, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, nil
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	found := map[string]Repo{}
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			return
		}
		if !hasDocket(absPath) {
			return
		}
		found[absPath] = Repo{
			Name: filepath.Base(absPath),
			Path: absPath,
		}
	}

	add(absRoot)

	if submodulePaths, err := parseGitmodules(absRoot); err == nil {
		for _, relPath := range submodulePaths {
			add(filepath.Join(absRoot, relPath))
		}
	}

	_ = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(absRoot, path)
		if relErr != nil {
			return nil
		}
		depth := 0
		if rel != "." {
			depth = strings.Count(filepath.ToSlash(rel), "/") + 1
		}
		if depth > 2 {
			return filepath.SkipDir
		}
		if path != absRoot && hasDocket(path) {
			add(path)
			return filepath.SkipDir
		}
		return nil
	})

	repos := make([]Repo, 0, len(found))
	for _, repo := range found {
		repos = append(repos, repo)
	}
	sort.Slice(repos, func(i, j int) bool {
		left, _ := filepath.Rel(absRoot, repos[i].Path)
		right, _ := filepath.Rel(absRoot, repos[j].Path)
		return left < right
	})
	return repos, nil
}

func hasDocket(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".docket"))
	return err == nil && info.IsDir()
}

func parseGitmodules(root string) ([]string, error) {
	file, err := os.Open(filepath.Join(root, ".gitmodules"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var paths []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "path") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		path := strings.TrimSpace(parts[1])
		if path != "" {
			paths = append(paths, path)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return paths, nil
}
