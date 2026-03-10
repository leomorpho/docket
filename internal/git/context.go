package git

import (
	"fmt"
	"strconv"
	"strings"
)

type LineBlame struct {
	SHA  string
	Line int
}

// BlameFile runs git blame --porcelain for a file (optionally line range) and returns line->commit associations.
func BlameFile(repoRoot, file string, startLine, endLine int) ([]LineBlame, error) {
	args := []string{"blame", "--porcelain"}
	if startLine > 0 && endLine >= startLine {
		args = append(args, "-L", fmt.Sprintf("%d,%d", startLine, endLine))
	}
	args = append(args, file)

	out, err := runGit(repoRoot, args...)
	if err != nil {
		return nil, fmt.Errorf("running git blame: %w", err)
	}

	var result []LineBlame
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		if len(parts[0]) != 40 {
			continue
		}
		finalLine, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}
		result = append(result, LineBlame{SHA: parts[0], Line: finalLine})
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no blame entries found for %s", file)
	}

	return result, nil
}
