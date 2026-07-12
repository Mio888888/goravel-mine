package main

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	cloneWindowLines = 8
	cloneWindowChars = 80
)

type cloneWindow struct {
	File      int
	Start     int
	End       int
	StartLine int
	EndLine   int
	Chars     int
	Hash      string
}

type cloneBlock struct {
	Chars     int
	Locations []cloneLocation
}

type cloneGroup struct {
	Windows []cloneWindow
}

func detectClones(files []preparedFile) (int, int, []cloneSample) {
	windows := collectCloneWindows(files)
	blocks := buildCloneBlocks(files, windows)
	duplicated := countDuplicatedLines(files, blocks)
	samples := buildCloneSamples(files, blocks)
	return len(blocks), duplicated, samples
}

func collectCloneWindows(files []preparedFile) map[string][]cloneWindow {
	windows := map[string][]cloneWindow{}
	for index, file := range files {
		items := file.Normalized
		for start := 0; start+cloneWindowLines <= len(items); start++ {
			window := makeCloneWindow(index, items, start)
			if window.Chars < cloneWindowChars {
				continue
			}
			windows[window.Hash] = append(windows[window.Hash], window)
		}
	}
	return windows
}

func makeCloneWindow(fileIndex int, items []normalizedLine, start int) cloneWindow {
	end := start + cloneWindowLines
	lines := make([]string, 0, cloneWindowLines)
	chars := 0
	for _, item := range items[start:end] {
		lines = append(lines, item.Text)
		chars += len(item.Text)
	}
	payload := strings.Join(lines, "\n")
	sum := sha256.Sum256([]byte(payload))
	return cloneWindow{
		File:      fileIndex,
		Start:     start,
		End:       end - 1,
		StartLine: items[start].Number,
		EndLine:   items[end-1].Number,
		Chars:     chars,
		Hash:      hex.EncodeToString(sum[:]),
	}
}

func buildCloneBlocks(files []preparedFile, windows map[string][]cloneWindow) []cloneBlock {
	hashes := orderedHashes(windows)
	groups := make([]cloneGroup, 0, len(hashes))
	for _, hash := range hashes {
		items := nonOverlappingWindows(files, windows[hash])
		if len(items) >= 2 {
			groups = append(groups, cloneGroup{Windows: items})
		}
	}
	blocks := mergeAdjacentGroups(files, groups)
	sort.Slice(blocks, func(i, j int) bool {
		left := blocks[i].Locations[0]
		right := blocks[j].Locations[0]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		return left.StartLine < right.StartLine
	})
	return blocks
}

func orderedHashes(windows map[string][]cloneWindow) []string {
	hashes := make([]string, 0, len(windows))
	for hash := range windows {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	return hashes
}

func nonOverlappingWindows(files []preparedFile, windows []cloneWindow) []cloneWindow {
	sort.Slice(windows, func(i, j int) bool {
		if files[windows[i].File].File.Path != files[windows[j].File].File.Path {
			return files[windows[i].File].File.Path < files[windows[j].File].File.Path
		}
		return windows[i].StartLine < windows[j].StartLine
	})
	var selected []cloneWindow
	for _, window := range windows {
		if overlapsWindow(selected, window) {
			continue
		}
		selected = append(selected, window)
	}
	return selected
}

func overlapsWindow(windows []cloneWindow, target cloneWindow) bool {
	for _, window := range windows {
		if window.File == target.File && target.Start <= window.End && target.End >= window.Start {
			return true
		}
	}
	return false
}

func mergeAdjacentGroups(files []preparedFile, groups []cloneGroup) []cloneBlock {
	sort.Slice(groups, func(i, j int) bool {
		return groupPositionKey(files, groups[i]) < groupPositionKey(files, groups[j])
	})
	used := make([]bool, len(groups))
	blocks := make([]cloneBlock, 0, len(groups))
	for index := range groups {
		if used[index] {
			continue
		}
		current := groups[index]
		for {
			next := adjacentGroupIndex(current, groups, used)
			if next < 0 {
				break
			}
			used[next] = true
			current = extendCloneGroup(files, current, groups[next])
		}
		blocks = append(blocks, cloneGroupBlock(files, current))
	}
	return blocks
}

func groupPositionKey(files []preparedFile, group cloneGroup) string {
	parts := make([]string, 0, len(group.Windows))
	for _, window := range group.Windows {
		parts = append(parts, files[window.File].File.Path+":"+formatCloneIndex(window.Start))
	}
	return strings.Join(parts, "|")
}

func formatCloneIndex(value int) string {
	const width = 10
	text := "0000000000" + strconv.Itoa(value)
	return text[len(text)-width:]
}

func adjacentGroupIndex(current cloneGroup, groups []cloneGroup, used []bool) int {
	for index, candidate := range groups {
		if used[index] || len(candidate.Windows) != len(current.Windows) {
			continue
		}
		matches := true
		for locationIndex, window := range current.Windows {
			next := candidate.Windows[locationIndex]
			expectedStart := window.End - cloneWindowLines + 2
			if window.File != next.File || next.Start != expectedStart {
				matches = false
				break
			}
		}
		if matches {
			return index
		}
	}
	return -1
}

func extendCloneGroup(files []preparedFile, current, next cloneGroup) cloneGroup {
	for index := range current.Windows {
		newLine := files[next.Windows[index].File].Normalized[next.Windows[index].End]
		current.Windows[index].End = next.Windows[index].End
		current.Windows[index].EndLine = next.Windows[index].EndLine
		current.Windows[index].Chars += len(newLine.Text)
	}
	return current
}

func cloneGroupBlock(files []preparedFile, group cloneGroup) cloneBlock {
	locations := make([]cloneLocation, 0, len(group.Windows))
	for _, window := range group.Windows {
		locations = append(locations, cloneLocation{
			Path:      files[window.File].File.Path,
			StartLine: window.StartLine,
			EndLine:   window.EndLine,
		})
	}
	return cloneBlock{Chars: group.Windows[0].Chars, Locations: locations}
}

func normalizeLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || isCommentLine(trimmed) || isImportLine(trimmed) || isPackageLine(trimmed) {
		return "", false
	}
	normalized := strings.Join(strings.Fields(trimmed), " ")
	if normalized == "" || punctuationOnly(normalized) {
		return "", false
	}
	return normalized, true
}

func isCommentLine(line string) bool {
	return strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "*/")
}

func isImportLine(line string) bool {
	return strings.HasPrefix(line, "import ") || line == "import(" || line == "import (" || line == ")"
}

func isPackageLine(line string) bool {
	return strings.HasPrefix(line, "package ")
}

func punctuationOnly(line string) bool {
	for _, r := range line {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
