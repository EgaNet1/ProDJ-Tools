// Package naming handles the set numbers ProDJ Tools puts in front of file
// names ("07 - Artist - Title.flac") and titles ("07 Title").
package naming

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var filePrefix = regexp.MustCompile(`^(\d{1,4}) - `)

// Strip removes the set number: "07 - A - B.flac" becomes "A - B.flac".
func Strip(name string) string { return filePrefix.ReplaceAllString(name, "") }

// Number returns the set number at the start of a file name, or 0.
func Number(name string) int {
	m := filePrefix.FindStringSubmatch(name)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// Width is how many digits numbers in a set of count tracks need (at least
// two, so that files sort correctly by name).
func Width(count int) int { return max(2, len(strconv.Itoa(count))) }

// File gives a file name its position in the set.
func File(n, width int, name string) string {
	return fmt.Sprintf("%0*d - %s", width, n, Strip(name))
}

// Title gives a track title its position in the set.
func Title(n, width int, title string) string {
	return fmt.Sprintf("%0*d %s", width, n, title)
}

// CleanTitle undoes Title. It removes the number only when it equals the
// file's own set number n, so a song really called "99 Problems" keeps its name.
func CleanTitle(title string, n int) string {
	if n == 0 {
		return title
	}
	num, rest, ok := strings.Cut(title, " ")
	if v, err := strconv.Atoi(num); ok && err == nil && v == n && rest != "" {
		return rest
	}
	return title
}
