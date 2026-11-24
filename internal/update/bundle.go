// Package update verifies and stages 0c release bundles before any installer runs.
package update

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const MaxArchive = int64(1 << 30)
const MaxExpanded = int64(2 << 30)

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$`)
var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var digitsPattern = regexp.MustCompile(`^[0-9]+$`)
var devicePattern = regexp.MustCompile(`^(COM|LPT)[1-9]$`)

func ValidVersion(v string) bool {
	if !versionPattern.MatchString(v) {
		return false
	}
	parts := strings.SplitN(v, "-", 2)
	if len(parts) == 2 {
		for _, p := range strings.Split(parts[1], ".") {
			if digitsPattern.MatchString(p) && len(p) > 1 && p[0] == '0' {
				return false
			}
		}
	}
	return true
}

type Release struct {
	Schema  int               `json:"schema"`
	Product string            `json:"product"`
	Version string            `json:"version"`
	Channel string            `json:"channel"`
	OS      string            `json:"os"`
	Arch    string            `json:"arch"`
	Files   map[string]string `json:"files"`
}

func Hash(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// SafeName is portable to Windows: no traversal, ADS, device names or ambiguous casing aliases.
func SafeName(name string) bool {
	if name == "" || strings.ContainsAny(name, `\:`) || strings.HasPrefix(name, "/") || path.Clean(name) != name {
		return false
	}
	for _, component := range strings.Split(name, "/") {
		if component == "." || component == ".." || strings.TrimRight(component, ". ") != component {
			return false
		}
		for _, c := range component {
			if c < 32 || strings.ContainsRune(`<>"|?*`, c) {
				return false
			}
		}
		base := strings.ToUpper(strings.SplitN(component, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || devicePattern.MatchString(base) {
			return false
		}
	}
	return true
}

func regular(root, name string) (string, error) {
	if !SafeName(name) {
		return "", fmt.Errorf("unsafe bundle path: %q", name)
	}
	current := root
	parts := strings.Split(name, "/")
	for i, part := range parts {
		current = filepath.Join(current, part)
		st, err := os.Lstat(current)
		if err != nil {
			return "", err
		}
		if st.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !st.IsDir()) {
			return "", fmt.Errorf("bundle path is a link or not a directory: %s", current)
		}
		if i == len(parts)-1 && !st.Mode().IsRegular() {
			return "", fmt.Errorf("not a regular file: %s", current)
		}
	}
	return current, nil
}

func Verify(root, targetOS, targetArch string) (*Release, error) {
	manifest, err := regular(root, "release.json")
	if err != nil {
		return nil, err
	}
	f, err := os.Open(manifest)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 1<<20 {
		return nil, fmt.Errorf("release metadata too large")
	}
	var r Release
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	if r.Schema != 1 || r.Product != "0c" || !ValidVersion(r.Version) {
		return nil, fmt.Errorf("unsupported release metadata")
	}
	if r.OS != targetOS || r.Arch != targetArch {
		return nil, fmt.Errorf("bundle targets %s/%s, need %s/%s", r.OS, r.Arch, targetOS, targetArch)
	}
	if r.Channel != "public-beta" && r.Channel != "internal-preview" {
		return nil, fmt.Errorf("unknown release channel")
	}
	data := []string{"manifest.json", "lexicon.bin", "lm.bin", "NOTICE.txt", "report.txt"}
	var required []string
	for _, n := range data {
		required = append(required, "data/"+n)
	}
	if targetOS != "windows" {
		return nil, fmt.Errorf("unsupported platform: %s", targetOS)
	}
	layout, err := WindowsLayoutOf(r.Files)
	if err != nil {
		return nil, err
	}
	required = append(required, "0c-update.exe", layout.DLL, layout.Server, layout.UI)
	for _, n := range required {
		if _, ok := r.Files[n]; !ok {
			return nil, fmt.Errorf("missing bundle file: %s", n)
		}
	}
	seen := map[string]bool{}
	for name, expected := range r.Files {
		key := strings.ToLower(name)
		if key == "release.json" || key == "sha256sums" || seen[key] || !hashPattern.MatchString(expected) {
			return nil, fmt.Errorf("duplicate path or invalid hash: %s", name)
		}
		seen[key] = true
		p, err := regular(root, name)
		if err != nil {
			return nil, err
		}
		actual, err := Hash(p)
		if err != nil {
			return nil, err
		}
		if actual != expected {
			return nil, fmt.Errorf("hash mismatch: %s", name)
		}
	}
	// Reject extras that could change DLL search or script imports after verification.
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		name := filepath.ToSlash(rel)
		if !d.Type().IsRegular() {
			return fmt.Errorf("non-regular bundle entry: %s", name)
		}
		if name != "release.json" && name != "SHA256SUMS" && !seen[strings.ToLower(name)] {
			return fmt.Errorf("unlisted bundle file: %s", name)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Stage requires a checksum from a trusted release feed or an explicit user argument.
// It never runs code and only writes under a new private temporary directory (never next to the archive,
// which may be on a USB stick or in any folder).
func Stage(archive, expected, targetOS, targetArch string) (string, func(), error) {
	noop := func() {}
	if !hashPattern.MatchString(expected) {
		return "", noop, fmt.Errorf("a lowercase SHA-256 is required")
	}
	st, err := os.Stat(archive)
	if err != nil {
		return "", noop, err
	}
	if st.Size() > MaxArchive {
		return "", noop, fmt.Errorf("archive too large")
	}
	actual, err := Hash(archive)
	if err != nil {
		return "", noop, err
	}
	if actual != expected {
		return "", noop, fmt.Errorf("archive SHA-256 mismatch")
	}
	temp, err := os.MkdirTemp("", stagePrefix)
	if err != nil {
		return "", noop, err
	}
	cleanup := func() { os.RemoveAll(temp) }
	if err = extract(archive, temp); err != nil {
		cleanup()
		return "", noop, err
	}
	entries, err := os.ReadDir(temp)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		cleanup()
		return "", noop, fmt.Errorf("archive must contain one root directory")
	}
	root := filepath.Join(temp, entries[0].Name())
	if _, err = Verify(root, targetOS, targetArch); err != nil {
		cleanup()
		return "", noop, err
	}
	return root, cleanup, nil
}

const stagePrefix = "0c-update-"

func extract(archive, root string) error {
	seen := map[string]bool{}
	var total int64
	count := 0
	put := func(name string, size int64, mode os.FileMode, directory bool, content io.Reader) error {
		name = strings.TrimSuffix(name, "/")
		count++
		if !SafeName(name) || count > 10000 {
			return fmt.Errorf("unsafe or excessive archive entries: %q", name)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("duplicate archive entry: %s", name)
		}
		seen[key] = true
		p := filepath.Join(root, filepath.FromSlash(name))
		if directory {
			return os.MkdirAll(p, 0700)
		}
		if size < 0 || size > MaxArchive || total > MaxExpanded-size {
			return fmt.Errorf("expanded bundle too large")
		}
		total += size
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			return err
		}
		perm := os.FileMode(0600)
		if mode&0111 != 0 {
			perm = 0700
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
		if err != nil {
			return err
		}
		_, err = io.CopyN(f, content, size)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Mode()&os.ModeSymlink != 0 || (!f.FileInfo().IsDir() && !f.Mode().IsRegular()) {
			return fmt.Errorf("archive links/devices are not allowed")
		}
		if f.UncompressedSize64 > uint64(MaxArchive) {
			return fmt.Errorf("archive member too large")
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		err = put(f.Name, int64(f.UncompressedSize64), f.Mode(), f.FileInfo().IsDir(), r)
		r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
