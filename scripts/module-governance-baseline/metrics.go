package main

import (
	"bufio"
	"os"
	"sort"
	"strings"
)

type preparedFile struct {
	File       scopedFile
	Lines      int
	Normalized []normalizedLine
}

type normalizedLine struct {
	Number int
	Text   string
}

func collectMetrics(files []scopedFile) ([]preparedFile, []groupReport, int, int, []fileSummary, error) {
	prepared := make([]preparedFile, 0, len(files))
	groupStats := map[string]*groupReport{}
	var totalFiles int
	var totalLines int
	for _, file := range files {
		item, err := prepareFile(file)
		if err != nil {
			return nil, nil, 0, 0, nil, err
		}
		prepared = append(prepared, item)
		totalFiles++
		totalLines += item.Lines
		updateGroup(groupStats, file.Group, item.Lines)
	}
	groups := sortedGroups(groupStats)
	largest := sortedLargest(prepared)
	return prepared, groups, totalFiles, totalLines, largest, nil
}

func prepareFile(file scopedFile) (preparedFile, error) {
	handle, err := os.Open(file.Full)
	if err != nil {
		return preparedFile{}, err
	}
	defer handle.Close()

	scanner := bufio.NewScanner(handle)
	lines := 0
	var normalized []normalizedLine
	state := lineNormalizationState{}
	for scanner.Scan() {
		lines++
		text, ok := state.normalize(scanner.Text())
		if ok {
			normalized = append(normalized, normalizedLine{
				Number: lines,
				Text:   text,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return preparedFile{}, err
	}
	return preparedFile{
		File:       file,
		Lines:      lines,
		Normalized: normalized,
	}, nil
}

type lineNormalizationState struct {
	inBlockComment bool
	inImportBlock  bool
}

func (s *lineNormalizationState) normalize(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if s.inBlockComment {
		s.inBlockComment = !strings.Contains(trimmed, "*/")
		return "", false
	}
	if strings.HasPrefix(trimmed, "/*") {
		s.inBlockComment = !strings.Contains(trimmed, "*/")
		return "", false
	}
	if s.inImportBlock {
		s.inImportBlock = trimmed != ")"
		return "", false
	}
	if trimmed == "import(" || trimmed == "import (" {
		s.inImportBlock = true
		return "", false
	}
	return normalizeLine(trimmed)
}

func updateGroup(groups map[string]*groupReport, name string, lines int) {
	entry, exists := groups[name]
	if !exists {
		entry = &groupReport{Name: name}
		groups[name] = entry
	}
	entry.Files++
	entry.Lines += lines
}

func sortedGroups(groups map[string]*groupReport) []groupReport {
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]groupReport, 0, len(names))
	for _, name := range names {
		result = append(result, *groups[name])
	}
	return result
}

func sortedLargest(files []preparedFile) []fileSummary {
	summaries := make([]fileSummary, 0, len(files))
	for _, file := range files {
		summaries = append(summaries, fileSummary{
			Path:  file.File.Path,
			Group: file.File.Group,
			Lines: file.Lines,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Lines != summaries[j].Lines {
			return summaries[i].Lines > summaries[j].Lines
		}
		return summaries[i].Path < summaries[j].Path
	})
	if len(summaries) > largestFileLimit {
		summaries = summaries[:largestFileLimit]
	}
	return summaries
}

func countDuplicatedLines(files []preparedFile, blocks []cloneBlock) int {
	index := preparedFileIndex(files)
	perFile := map[string]map[int]struct{}{}
	for _, block := range blocks {
		for _, location := range block.Locations {
			lines := perFile[location.Path]
			if lines == nil {
				lines = map[int]struct{}{}
				perFile[location.Path] = lines
			}
			for _, line := range cloneLineNumbers(index[location.Path], location) {
				lines[line] = struct{}{}
			}
		}
	}
	total := 0
	for _, lines := range perFile {
		total += len(lines)
	}
	return total
}

func buildCloneSamples(files []preparedFile, blocks []cloneBlock) []cloneSample {
	index := preparedFileIndex(files)
	limit := min(len(blocks), cloneSampleLimit)
	samples := make([]cloneSample, 0, limit)
	for _, block := range blocks[:limit] {
		lines := len(cloneLineNumbers(index[block.Locations[0].Path], block.Locations[0]))
		samples = append(samples, cloneSample{
			Lines: lines, Chars: block.Chars, Locations: block.Locations,
		})
	}
	return samples
}

func preparedFileIndex(files []preparedFile) map[string]preparedFile {
	index := make(map[string]preparedFile, len(files))
	for _, file := range files {
		index[file.File.Path] = file
	}
	return index
}

func cloneLineNumbers(file preparedFile, location cloneLocation) []int {
	var lines []int
	for _, line := range file.Normalized {
		if line.Number >= location.StartLine && line.Number <= location.EndLine {
			lines = append(lines, line.Number)
		}
	}
	return lines
}
