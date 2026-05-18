package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	stdhtml "html"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ─── Data Types ───────────────────────────────────────────────────────────────

type Link struct {
	Name   string   `yaml:"name"   json:"name"`
	Href   string   `yaml:"href"   json:"href"`
	Env    string   `yaml:"env,omitempty"    json:"env,omitempty"`
	Desc   string   `yaml:"desc,omitempty"   json:"desc,omitempty"`
	Tags   []string `yaml:"tags,omitempty"   json:"tags,omitempty"`
	Pinned bool     `yaml:"pinned,omitempty" json:"pinned,omitempty"`
}

type Section struct {
	ID    string `yaml:"id"    json:"id"`
	Label string `yaml:"label" json:"label"`
	Tag   string `yaml:"tag"   json:"tag"`
	Links []Link `yaml:"links" json:"links"`
}

type Config struct {
	Title    string    `yaml:"title"    json:"title"`
	Sections []Section `yaml:"sections" json:"sections"`
}

type PageData struct {
	Config Config
	CSS    template.CSS
}

// ─── Bookmark Regexes ─────────────────────────────────────────────────────────

var (
	reH3     = regexp.MustCompile(`(?i)<H3[^>]*>(.*?)</H3>`)
	reA      = regexp.MustCompile(`(?i)<A\s[^>]*HREF="([^"]*)"[^>]*>(.*?)</A>`)
	reDLOpen = regexp.MustCompile(`(?i)<DL\b`)
	reDLEnd  = regexp.MustCompile(`(?i)</DL\b`)
)

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	inputFlag := flag.String("input", "", "Input file: config.json or Chrome bookmarks.html (required)")
	outFlag := flag.String("out", "wink.html", "Output HTML file")
	tmplFlag := flag.String("template", "aio.html", "Template file")
	cssFlag := flag.String("css", "styles.css", "CSS file to inline")
	flag.Parse()

	if *inputFlag == "" {
		flag.Usage()
		os.Exit(1)
	}

	inputData, err := os.ReadFile(*inputFlag)
	onErr(err, "reading input")

	var config Config
	switch strings.ToLower(filepath.Ext(*inputFlag)) {
	case ".json":
		err = json.Unmarshal(inputData, &config)
		onErr(err, "parsing JSON config")
	case ".html", ".htm":
		config, err = parseBookmarks(inputData)
		onErr(err, "parsing bookmarks HTML")
	default:
		log.Fatalf("unrecognised input format — expected .json or .html")
	}

	cssData, err := os.ReadFile(*cssFlag)
	onErr(err, "reading CSS")

	tmplData, err := os.ReadFile(*tmplFlag)
	onErr(err, "reading template")

	tmpl, err := template.New("page").Parse(string(tmplData))
	onErr(err, "parsing template")

	outFile, err := os.Create(*outFlag)
	onErr(err, "creating output file")
	defer outFile.Close()

	err = tmpl.Execute(outFile, PageData{Config: config, CSS: template.CSS(cssData)})
	onErr(err, "executing template")

	total := 0
	for _, s := range config.Sections {
		total += len(s.Links)
	}
	log.Printf("wrote %s — %d sections, %d links", *outFlag, len(config.Sections), total)
}

func onErr(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %v", msg, err)
	}
}

// ─── Bookmarks Parser ─────────────────────────────────────────────────────────
//
// Chrome's export is the legacy Netscape bookmark format — one logical element
// per line, so a line-by-line scanner + targeted regexes is both simpler and
// sufficient.  The structure we care about:
//
//   <DL><p>
//     <DT><H3>Folder name</H3>        ← folder header
//     <DL><p>                         ← folder contents follow immediately
//       <DT><A HREF="...">Name</A>    ← bookmark
//       <DT><H3>Sub-folder</H3>       ← nested folder (also becomes a section)
//       <DL><p> ... </DL>
//     </DL>
//   </DL>
//
// We maintain an integer stack of section indices rather than *Section pointers
// because appending to the sections slice can reallocate the backing array,
// invalidating any stored pointer.

func parseBookmarks(data []byte) (Config, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))

	var sections []Section
	// idxStack tracks which section was active before we entered each <DL>.
	// -1 means "no active section" (e.g. the root DL level).
	idxStack := []int{}
	curIdx := -1

	// pendingFolder holds a folder name read from an <H3> while waiting for
	// the <DL> that is its content container — always the next sibling in
	// Chrome's format.
	pendingFolder := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		switch {
		case reDLEnd.MatchString(line):
			if len(idxStack) > 0 {
				curIdx = idxStack[len(idxStack)-1]
				idxStack = idxStack[:len(idxStack)-1]
			} else {
				curIdx = -1
			}

		case reDLOpen.MatchString(line):
			if pendingFolder != "" {
				sections = append(sections, Section{
					ID:    uniqueSectionID(pendingFolder, sections),
					Label: pendingFolder,
					Tag:   guessTag(pendingFolder),
				})
				idxStack = append(idxStack, curIdx)
				curIdx = len(sections) - 1
				pendingFolder = ""
			} else {
				// Root DL or unexpected DL — push a sentinel so </DL> can pop.
				idxStack = append(idxStack, curIdx)
			}

		default:
			if m := reH3.FindStringSubmatch(line); m != nil {
				pendingFolder = stdhtml.UnescapeString(m[1])
				continue
			}
			if m := reA.FindStringSubmatch(line); m != nil {
				href := m[1]
				name := stdhtml.UnescapeString(m[2])
				if curIdx >= 0 && isHTTP(href) && name != "" {
					sections[curIdx].Links = append(sections[curIdx].Links, Link{
						Name: name,
						Href: href,
					})
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return Config{}, err
	}

	// Drop sections that ended up with no links (e.g. pure container folders
	// whose only contents were sub-folders, which became their own sections).
	result := sections[:0]
	for _, s := range sections {
		if len(s.Links) > 0 {
			result = append(result, s)
		}
	}

	return Config{Title: "Bookmarks", Sections: result}, nil
}

// isHTTP rejects bookmarklets (javascript:) and other non-navigable schemes.
func isHTTP(href string) bool {
	return strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = nonAlnum.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// uniqueSectionID guarantees no two sections share the same DOM id.
func uniqueSectionID(label string, existing []Section) string {
	base := "sec-" + slugify(label)
	if base == "sec-" {
		base = "sec-folder"
	}
	candidate := base
	for n := 2; ; n++ {
		taken := false
		for _, s := range existing {
			if s.ID == candidate {
				taken = true
				break
			}
		}
		if !taken {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, n)
	}
}

// guessTag maps common folder name keywords to one of the CSS tag classes
// defined in styles.css.  Falls back to tag-internal for anything unknown.
func guessTag(name string) string {
	lower := strings.ToLower(name)
	switch {
	case containsAny(lower, "dev", "code", "git", "engineer", "tech", "tool", "util"):
		return "tag-tools"
	case containsAny(lower, "news", "media", "read", "blog", "article", "comm", "mail"):
		return "tag-comms"
	case containsAny(lower, "admin", "ops", "infra", "manage", "settings"):
		return "tag-admin"
	case containsAny(lower, "client", "customer", "partner", "vendor"):
		return "tag-client"
	default:
		return "tag-internal"
	}
}

func containsAny(s string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
