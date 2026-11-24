package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// defaultCategories maps a lowercase extension (without the dot) to its
// folder name. The table is deliberately small and memorable; users can
// extend it with a JSON config file (see LoadRules).
var defaultCategories = map[string]string{
	// Images
	"jpg": "images", "jpeg": "images", "png": "images", "gif": "images",
	"webp": "images", "svg": "images", "bmp": "images", "tiff": "images",
	"heic": "images", "ico": "images", "avif": "images",
	// Documents
	"pdf": "documents", "doc": "documents", "docx": "documents",
	"txt": "documents", "md": "documents", "rtf": "documents",
	"odt": "documents", "epub": "documents",
	// Spreadsheets
	"xls": "spreadsheets", "xlsx": "spreadsheets", "csv": "spreadsheets",
	"ods": "spreadsheets",
	// Presentations
	"ppt": "presentations", "pptx": "presentations", "odp": "presentations",
	"key": "presentations",
	// Audio
	"mp3": "audio", "wav": "audio", "flac": "audio", "ogg": "audio",
	"aac": "audio", "m4a": "audio", "opus": "audio",
	// Video
	"mp4": "video", "mkv": "video", "mov": "video", "avi": "video",
	"webm": "video", "wmv": "video",
	// Archives
	"zip": "archives", "tar": "archives", "gz": "archives", "bz2": "archives",
	"xz": "archives", "7z": "archives", "rar": "archives",
	// Code
	"go": "code", "py": "code", "js": "code", "ts": "code", "java": "code",
	"c": "code", "cpp": "code", "h": "code", "cs": "code", "rs": "code",
	"rb": "code", "php": "code", "sh": "code", "html": "code",
	"css": "code", "json": "code", "yaml": "code", "yml": "code",
	"toml": "code", "sql": "code",
	// Fonts
	"ttf": "fonts", "otf": "fonts", "woff": "fonts", "woff2": "fonts",
	// Executables / installers
	"exe": "installers", "msi": "installers", "dmg": "installers",
	"deb": "installers", "apk": "installers", "appimage": "installers",
}

// categoryOrder fixes the display order of the built-in categories.
var categoryOrder = []string{
	"images", "documents", "spreadsheets", "presentations", "audio",
	"video", "archives", "code", "fonts", "installers", "other",
}

// classifier resolves an extension to a folder name, honoring user rules
// first and falling back to the built-in table.
type classifier struct {
	rules map[string]string // extension (lowercase, no dot) -> category
}

func newClassifier(extra map[string]string) *classifier {
	rules := make(map[string]string, len(defaultCategories)+len(extra))
	for ext, cat := range defaultCategories {
		rules[ext] = cat
	}
	for ext, cat := range extra {
		rules[strings.ToLower(strings.TrimPrefix(ext, "."))] = cat
	}
	return &classifier{rules: rules}
}

// category returns the folder name for a file name. Unknown extensions and
// extensionless files go to "other".
func (c *classifier) category(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if ext == "" {
		return "other"
	}
	if cat, ok := c.rules[ext]; ok {
		return cat
	}
	return "other"
}

// LoadRules reads a JSON object of extension -> category pairs, e.g.
// {"blend": "3d-models", "psd": "design"}. Keys may include a leading dot.
func LoadRules(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var rules map[string]string
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	for ext, cat := range rules {
		if strings.TrimSpace(cat) == "" {
			return nil, fmt.Errorf("config %s: empty category for extension %q", path, ext)
		}
	}
	return rules, nil
}

// printCategories lists the built-in extension table grouped by category.
func printCategories() {
	byCat := map[string][]string{}
	for ext, cat := range defaultCategories {
		byCat[cat] = append(byCat[cat], ext)
	}
	for _, cat := range categoryOrder {
		exts := byCat[cat]
		if cat == "other" {
			fmt.Println("other         (unknown extensions and extensionless files)")
			continue
		}
		sort.Strings(exts)
		fmt.Printf("%-13s %s\n", cat, strings.Join(exts, ", "))
	}
}
